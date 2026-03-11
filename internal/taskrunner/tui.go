package taskrunner

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// agentPaneState holds the per-agent TUI state: active card, tool calls, text output.
type agentPaneState struct {
	agentName string

	// Card lifecycle
	activeCard *cardState
	phase      string // "starting", "polling", "running", "idle", "stopped"

	// Tool calls and output
	toolCalls  []toolCallEntry
	activeCall *toolCallEntry
	textLines  []string
	stats      sessionStats

	// File tracking
	filesRead   []string
	filesEdited []string

	// Viewports (one per pane)
	toolViewport     viewport.Model
	textViewport     viewport.Model
	toolContentWidth int
	textContentWidth int
}

func newAgentPaneState(agentName string) *agentPaneState {
	return &agentPaneState{
		agentName: agentName,
		phase:     "starting",
	}
}

type toolCallEntry struct {
	toolName   string
	summary    string // e.g. "main.go" for Read, "go test ./..." for Bash
	durationMs int    // -1 while in progress
	timestamp  time.Time
}

type sessionStats struct {
	inputTokens     int
	outputTokens    int
	cacheReadTokens int
	turns           int
}

// TUIModel is the Bubble Tea model for the devpilot run dashboard.
type TUIModel struct {
	// Config
	boardName string
	cancel    context.CancelFunc

	// Global state
	boardID string
	lists   []listState
	phase   string // overall phase: "starting", "polling", "running", "idle", "stopped"
	lastErr string
	history []cardState

	// Per-agent panes (ordered for rendering)
	agentOrder  []string
	agentPanes  map[string]*agentPaneState
	focusedAgent string // which agent's pane receives keyboard input
	focusedPane  string // "tools" or "text" within the focused agent

	// Layout
	width  int
	height int
	ready  bool

	// Event channel
	eventCh <-chan Event
}

type listState struct {
	name string
	id   string
}

type cardState struct {
	id       string
	name     string
	branch   string
	status   string // "running", "done", "failed"
	prURL    string
	errMsg   string
	duration time.Duration
	started  time.Time
	agent    string
}

type runnerDoneMsg struct{}

type tickMsg time.Time

func tickEvery() tea.Cmd {
	return tea.Every(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// NewTUIModel creates a new TUI model for the given board and agent names.
func NewTUIModel(boardName string, agentNames []string, eventCh <-chan Event, cancel context.CancelFunc) TUIModel {
	if len(agentNames) == 0 {
		agentNames = []string{"claude"}
	}
	panes := make(map[string]*agentPaneState, len(agentNames))
	for _, name := range agentNames {
		panes[name] = newAgentPaneState(name)
	}
	return TUIModel{
		boardName:    boardName,
		cancel:       cancel,
		phase:        "starting",
		eventCh:      eventCh,
		agentOrder:   agentNames,
		agentPanes:   panes,
		focusedAgent: agentNames[0],
		focusedPane:  "tools",
	}
}

// pane returns the agentPaneState for the given agent name, or nil if not found.
func (m *TUIModel) pane(agentName string) *agentPaneState {
	if agentName == "" && len(m.agentOrder) > 0 {
		return m.agentPanes[m.agentOrder[0]]
	}
	return m.agentPanes[agentName]
}

// focusedPaneState returns the currently focused agent's pane.
func (m *TUIModel) focusedPaneState() *agentPaneState {
	return m.agentPanes[m.focusedAgent]
}

func waitForEvent(ch <-chan Event) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return runnerDoneMsg{}
		}
		return event
	}
}

// Init implements tea.Model.
func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(waitForEvent(m.eventCh), tickEvery())
}

// Update implements tea.Model.
func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updatePaneLayout()
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			m.cancel()
			return m, tea.Quit
		case tea.KeyTab:
			m.cycleAgentFocus()
			return m, nil
		}
		switch msg.String() {
		case "q":
			m.cancel()
			return m, tea.Quit
		case "tab":
			m.cycleAgentFocus()
			return m, nil
		case "g":
			if p := m.focusedPaneState(); p != nil {
				if m.focusedPane == "tools" {
					p.toolViewport.GotoTop()
				} else {
					p.textViewport.GotoTop()
				}
			}
			return m, nil
		case "G":
			if p := m.focusedPaneState(); p != nil {
				if m.focusedPane == "tools" {
					p.toolViewport.GotoBottom()
				} else {
					p.textViewport.GotoBottom()
				}
			}
			return m, nil
		case "1", "2", "3", "4":
			// Direct agent pane selection (1-4)
			idx := int(msg.String()[0] - '1')
			if idx >= 0 && idx < len(m.agentOrder) {
				m.focusedAgent = m.agentOrder[idx]
			}
			return m, nil
		}
		// Delegate scroll keys to focused viewport
		if p := m.focusedPaneState(); p != nil {
			var cmd tea.Cmd
			if m.focusedPane == "tools" {
				p.toolViewport, cmd = p.toolViewport.Update(msg)
			} else {
				p.textViewport, cmd = p.textViewport.Update(msg)
			}
			return m, cmd
		}
		return m, nil

	case tickMsg:
		return m, tickEvery()

	case runnerDoneMsg:
		m.phase = "stopped"
		return m, nil

	case AgentRegisteredEvent:
		// Ensure pane exists for dynamically registered agents.
		if _, ok := m.agentPanes[msg.AgentName]; !ok {
			m.agentPanes[msg.AgentName] = newAgentPaneState(msg.AgentName)
			m.agentOrder = append(m.agentOrder, msg.AgentName)
		}
		return m, waitForEvent(m.eventCh)

	case RunnerStartedEvent:
		m.boardID = msg.BoardID
		m.lists = make([]listState, 0, len(msg.Lists))
		for name, id := range msg.Lists {
			m.lists = append(m.lists, listState{name: name, id: id})
		}
		if p := m.pane(msg.AgentName); p != nil {
			p.phase = "polling"
		}
		m.phase = "polling"
		return m, waitForEvent(m.eventCh)

	case PollingEvent:
		if p := m.pane(msg.AgentName); p != nil && p.phase != "running" {
			p.phase = "polling"
		}
		if m.phase != "running" {
			m.phase = "polling"
		}
		return m, waitForEvent(m.eventCh)

	case NoTasksEvent:
		if p := m.pane(msg.AgentName); p != nil {
			p.phase = "idle"
		}
		return m, waitForEvent(m.eventCh)

	case CardStartedEvent:
		if p := m.pane(msg.AgentName); p != nil {
			p.activeCard = &cardState{
				id:      msg.CardID,
				name:    msg.CardName,
				branch:  msg.Branch,
				status:  "running",
				started: time.Now(),
				agent:   msg.AgentName,
			}
			p.toolCalls = nil
			p.activeCall = nil
			p.textLines = nil
			p.stats = sessionStats{}
			p.filesRead = nil
			p.filesEdited = nil
			p.phase = "running"
		}
		m.phase = "running"
		return m, waitForEvent(m.eventCh)

	case ToolStartEvent:
		if p := m.pane(msg.AgentName); p != nil {
			summary := toolSummary(msg.ToolName, msg.Input)
			p.activeCall = &toolCallEntry{
				toolName:   msg.ToolName,
				summary:    summary,
				durationMs: -1,
				timestamp:  time.Now(),
			}
			if fp := extractFilePath(msg.Input); fp != "" {
				switch msg.ToolName {
				case "Read", "Grep", "Glob":
					p.filesRead = addUnique(p.filesRead, fp)
				case "Edit", "Write":
					p.filesEdited = addUnique(p.filesEdited, fp)
				}
			}
		}
		return m, waitForEvent(m.eventCh)

	case ToolResultEvent:
		if p := m.pane(msg.AgentName); p != nil {
			if p.activeCall != nil {
				p.activeCall.durationMs = msg.DurationMs
				p.toolCalls = append(p.toolCalls, *p.activeCall)
				p.activeCall = nil
			}
			p.toolViewport.SetContent(renderToolCallsListForPane(p))
			p.toolViewport.GotoBottom()
		}
		return m, waitForEvent(m.eventCh)

	case TextOutputEvent:
		if p := m.pane(msg.AgentName); p != nil {
			p.textLines = append(p.textLines, msg.Text)
			if len(p.textLines) > maxTextLines {
				p.textLines = p.textLines[len(p.textLines)-maxTextLines:]
			}
			p.wrapAndSetTextContent()
			p.textViewport.GotoBottom()
		}
		return m, waitForEvent(m.eventCh)

	case StatsUpdateEvent:
		if p := m.pane(msg.AgentName); p != nil {
			p.stats.inputTokens += msg.InputTokens
			p.stats.outputTokens += msg.OutputTokens
			p.stats.cacheReadTokens += msg.CacheReadTokens
			if msg.Turns > 0 {
				p.stats.turns = msg.Turns
			}
		}
		return m, waitForEvent(m.eventCh)

	case CardDoneEvent:
		entry := cardState{
			id:       msg.CardID,
			name:     msg.CardName,
			status:   "done",
			prURL:    msg.PRURL,
			duration: msg.Duration,
			agent:    msg.AgentName,
		}
		m.history = append(m.history, entry)
		if p := m.pane(msg.AgentName); p != nil {
			p.activeCard = nil
			p.phase = "polling"
		}
		return m, waitForEvent(m.eventCh)

	case CardFailedEvent:
		entry := cardState{
			id:       msg.CardID,
			name:     msg.CardName,
			status:   "failed",
			errMsg:   msg.ErrMsg,
			duration: msg.Duration,
			agent:    msg.AgentName,
		}
		m.history = append(m.history, entry)
		if p := m.pane(msg.AgentName); p != nil {
			p.activeCard = nil
			p.phase = "polling"
		}
		return m, waitForEvent(m.eventCh)

	case ReviewStartedEvent, ReviewDoneEvent, FixStartedEvent, FixDoneEvent:
		return m, waitForEvent(m.eventCh)

	case RunnerStoppedEvent:
		if p := m.pane(msg.AgentName); p != nil {
			p.phase = "stopped"
		}
		// Mark global phase stopped when all agents are stopped.
		allStopped := true
		for _, p := range m.agentPanes {
			if p.phase != "stopped" {
				allStopped = false
				break
			}
		}
		if allStopped {
			m.phase = "stopped"
		}
		return m, waitForEvent(m.eventCh)

	case RunnerErrorEvent:
		if msg.Err != nil {
			m.lastErr = msg.Err.Error()
		}
		return m, waitForEvent(m.eventCh)
	}

	return m, nil
}

// cycleAgentFocus advances focus to the next agent pane.
func (m *TUIModel) cycleAgentFocus() {
	if len(m.agentOrder) <= 1 {
		// Single agent: toggle tools/text panes
		if m.focusedPane == "tools" {
			m.focusedPane = "text"
		} else {
			m.focusedPane = "tools"
		}
		return
	}
	for i, name := range m.agentOrder {
		if name == m.focusedAgent {
			m.focusedAgent = m.agentOrder[(i+1)%len(m.agentOrder)]
			return
		}
	}
}

// updatePaneLayout recalculates viewport dimensions for all agent panes.
func (m *TUIModel) updatePaneLayout() {
	if len(m.agentOrder) == 0 {
		return
	}

	const panelChrome = 4

	if len(m.agentOrder) == 1 {
		// Single-agent: same layout as before
		toolsWidth := m.width - 30 - 1
		if toolsWidth < 30 {
			toolsWidth = 30
		}
		toolContentWidth := toolsWidth - panelChrome
		if toolContentWidth < 1 {
			toolContentWidth = 1
		}
		textContentWidth := m.width - panelChrome
		if textContentWidth < 1 {
			textContentWidth = 1
		}

		fixedOverhead := 19
		availableHeight := m.height - fixedOverhead
		if availableHeight < 2 {
			availableHeight = 2
		}
		toolVPHeight := availableHeight * 6 / 10
		textVPHeight := availableHeight - toolVPHeight
		if toolVPHeight < 1 {
			toolVPHeight = 1
		}
		if textVPHeight < 1 {
			textVPHeight = 1
		}

		p := m.agentPanes[m.agentOrder[0]]
		p.toolContentWidth = toolContentWidth
		p.textContentWidth = textContentWidth
		if !m.ready {
			p.toolViewport = viewport.New(toolContentWidth, toolVPHeight)
			p.textViewport = viewport.New(textContentWidth, textVPHeight)
			m.ready = true
		} else {
			p.toolViewport.Width = toolContentWidth
			p.toolViewport.Height = toolVPHeight
			p.textViewport.Width = textContentWidth
			p.textViewport.Height = textVPHeight
		}
		p.wrapAndSetTextContent()
		return
	}

	// Multi-agent layout: split width evenly for 2, stack vertically for 3+
	n := len(m.agentOrder)
	const agentHeaderHeight = 4
	const sharedOverhead = 5 // header + footer
	availableHeight := m.height - sharedOverhead
	if availableHeight < 2 {
		availableHeight = 2
	}

	if n == 2 {
		// Side by side
		halfWidth := m.width / 2
		paneHeight := availableHeight - agentHeaderHeight
		if paneHeight < 2 {
			paneHeight = 2
		}
		for i, name := range m.agentOrder {
			p := m.agentPanes[name]
			w := halfWidth
			if i == 1 {
				w = m.width - halfWidth
			}
			contentW := w - panelChrome
			if contentW < 1 {
				contentW = 1
			}
			p.toolContentWidth = contentW
			p.textContentWidth = contentW
			if !m.ready {
				p.toolViewport = viewport.New(contentW, paneHeight/2)
				p.textViewport = viewport.New(contentW, paneHeight/2)
			} else {
				p.toolViewport.Width = contentW
				p.toolViewport.Height = paneHeight / 2
				p.textViewport.Width = contentW
				p.textViewport.Height = paneHeight / 2
			}
			p.wrapAndSetTextContent()
		}
	} else {
		// Stacked vertical for 3+
		paneHeight := (availableHeight - agentHeaderHeight*n) / n
		if paneHeight < 2 {
			paneHeight = 2
		}
		contentW := m.width - panelChrome
		if contentW < 1 {
			contentW = 1
		}
		for _, name := range m.agentOrder {
			p := m.agentPanes[name]
			p.toolContentWidth = contentW
			p.textContentWidth = contentW
			if !m.ready {
				p.toolViewport = viewport.New(contentW, paneHeight)
				p.textViewport = viewport.New(contentW, paneHeight)
			} else {
				p.toolViewport.Width = contentW
				p.toolViewport.Height = paneHeight
				p.textViewport.Width = contentW
				p.textViewport.Height = paneHeight
			}
			p.wrapAndSetTextContent()
		}
	}
	m.ready = true
}

// View implements tea.Model (rendering in tui_view.go).
func (m TUIModel) View() string {
	if !m.ready {
		return "  Starting devpilot run..."
	}
	return m.renderView()
}

// renderToolCallsListForPane renders the tool calls list for a specific agent pane.
func renderToolCallsListForPane(p *agentPaneState) string {
	const fixedCols = 4 + 8 + 1 + 1 + 6
	summaryWidth := p.toolContentWidth - fixedCols
	if summaryWidth < 10 {
		summaryWidth = 10
	}

	var lines []string
	for _, tc := range p.toolCalls {
		lines = append(lines, fmtToolCall("✓", tc, summaryWidth))
	}
	if p.activeCall != nil {
		lines = append(lines, fmtToolCall("⚡", *p.activeCall, summaryWidth))
	}
	return joinLines(lines)
}

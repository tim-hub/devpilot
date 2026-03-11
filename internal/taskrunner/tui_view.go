package taskrunner

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15")).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	activeCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("12")).
			Padding(0, 1)

	agentFocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("12")).
				Padding(0, 1)

	agentUnfocusedStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)

	doneIcon   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("✅")
	failedIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("❌")

	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	k := float64(n) / 1000.0
	if k < 10 {
		return fmt.Sprintf("%.1fk", k)
	}
	return fmt.Sprintf("%.0fk", k)
}

func formatDuration(ms int) string {
	if ms < 0 {
		return "..."
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}

func (m TUIModel) renderView() string {
	if m.width < 60 || m.height < 12 {
		return fmt.Sprintf("  Terminal too small (need 60x12, have %dx%d). Resize or use --no-tui.", m.width, m.height)
	}

	var sections []string
	sections = append(sections, renderHeader(m))

	if len(m.agentOrder) <= 1 {
		// Single-agent layout (backward compatible)
		sections = append(sections, renderStatusAndActive(m))
		sections = append(sections, renderToolsAndFiles(m))
		sections = append(sections, renderTextPane(m))
	} else if len(m.agentOrder) == 2 {
		// Two-agent side-by-side layout
		sections = append(sections, renderStatusRow(m))
		sections = append(sections, renderTwoAgentPanes(m))
	} else {
		// Three+ agents stacked vertically
		sections = append(sections, renderStatusRow(m))
		sections = append(sections, renderStackedAgentPanes(m))
	}

	sections = append(sections, renderFooter(m))
	return strings.Join(sections, "\n")
}

// --- Header ---

func renderHeader(m TUIModel) string {
	left := titleStyle.Render("devpilot run")
	middle := fmt.Sprintf(" Board: %s", m.boardName)

	// Show agent count if multiple
	if len(m.agentOrder) > 1 {
		middle += fmt.Sprintf(" • %d agents", len(m.agentOrder))
	}

	phaseText := m.phase
	switch m.phase {
	case "polling":
		phaseText = "polling..."
	case "running":
		phaseText = "▶ running"
	case "idle":
		phaseText = "waiting"
	case "stopped":
		phaseText = "■ stopped"
	}

	// Aggregate stats across all agents
	var totalIn, totalOut, totalTurns int
	for _, p := range m.agentPanes {
		totalIn += p.stats.inputTokens
		totalOut += p.stats.outputTokens
		if p.stats.turns > totalTurns {
			totalTurns = p.stats.turns
		}
	}
	statsText := ""
	if totalIn > 0 || totalOut > 0 {
		statsText = fmt.Sprintf(" ↑%s ↓%s", formatTokens(totalIn), formatTokens(totalOut))
	}
	if totalTurns > 0 {
		statsText += fmt.Sprintf(" T:%d", totalTurns)
	}

	right := fmt.Sprintf("[%s]%s [q: quit]", phaseText, statsText)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(middle) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return headerStyle.Width(m.width).Render(left + middle + strings.Repeat(" ", gap) + right)
}

// --- Single-agent layout (backward compatible) ---

func renderStatusPanel(m TUIModel) string {
	if len(m.lists) == 0 {
		return "(no lists)"
	}
	var lines []string
	for _, l := range m.lists {
		lines = append(lines, fmt.Sprintf("  %-14s", l.name))
	}
	return statusStyle.Render(strings.Join(lines, "\n"))
}

func renderActiveTask(m TUIModel) string {
	var p *agentPaneState
	if len(m.agentOrder) > 0 {
		p = m.agentPanes[m.agentOrder[0]]
	}

	if p == nil || p.activeCard == nil {
		phase := m.phase
		if p != nil {
			phase = p.phase
		}
		switch phase {
		case "idle":
			return activeCardStyle.Render("  (waiting for tasks...)")
		case "stopped":
			return activeCardStyle.Render("  (runner stopped)")
		default:
			return activeCardStyle.Render("  (polling...)")
		}
	}

	elapsed := time.Since(p.activeCard.started).Round(time.Second)
	lines := []string{
		fmt.Sprintf("  ▶ %q", p.activeCard.name),
		fmt.Sprintf("    Branch: %s", p.activeCard.branch),
		fmt.Sprintf("    Duration: %s", elapsed),
	}
	if p.activeCall != nil {
		lines = append(lines, fmt.Sprintf("    ⚡ %s %s", p.activeCall.toolName, p.activeCall.summary))
	}
	return activeCardStyle.Render(strings.Join(lines, "\n"))
}

func renderStatusAndActive(m TUIModel) string {
	status := renderStatusPanel(m)
	active := renderActiveTask(m)

	statusWidth := 22
	activeWidth := m.width - statusWidth - 1
	if activeWidth < 10 {
		activeWidth = 10
	}

	statusRendered := lipgloss.NewStyle().Width(statusWidth).Render(status)
	activeRendered := lipgloss.NewStyle().Width(activeWidth).Render(active)

	return lipgloss.JoinHorizontal(lipgloss.Top, statusRendered, " ", activeRendered)
}

func renderToolsAndFiles(m TUIModel) string {
	var p *agentPaneState
	if len(m.agentOrder) > 0 {
		p = m.agentPanes[m.agentOrder[0]]
	}
	if p == nil {
		return ""
	}

	toolsWidth := m.width - 30 - 1
	if toolsWidth < 30 {
		toolsWidth = 30
	}
	filesWidth := m.width - toolsWidth - 1

	toolsPanel := renderToolCallsPanelForPane(m, p, toolsWidth, m.focusedPane == "tools")
	filesPanel := renderFilesPanelForPane(p, filesWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, toolsPanel, " ", filesPanel)
}

func renderTextPane(m TUIModel) string {
	var p *agentPaneState
	if len(m.agentOrder) > 0 {
		p = m.agentPanes[m.agentOrder[0]]
	}
	if p == nil {
		return ""
	}

	focusColor := lipgloss.Color("240")
	if m.focusedPane == "text" {
		focusColor = lipgloss.Color("12")
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusColor).
		Width(m.width - 2).
		Padding(0, 1)

	label := agentOutputLabel(p.agentName)
	if len(p.textLines) == 0 {
		return style.Render(label + "\n  (no output yet)")
	}
	return style.Render(label + "\n" + p.textViewport.View())
}

func agentOutputLabel(name string) string {
	switch name {
	case "claude":
		return "Claude Output"
	case "gemini":
		return "Gemini Output"
	case "opencode":
		return "Opencode Output"
	case "cursor":
		return "Cursor Output"
	default:
		return name + " Output"
	}
}

// --- Multi-agent layout ---

// renderStatusRow renders a compact status bar showing Trello list info.
func renderStatusRow(m TUIModel) string {
	if len(m.lists) == 0 {
		return ""
	}
	var parts []string
	for _, l := range m.lists {
		parts = append(parts, l.name)
	}
	return statusStyle.Render("Lists: " + strings.Join(parts, " | "))
}

// renderTwoAgentPanes renders two agent panes side by side.
func renderTwoAgentPanes(m TUIModel) string {
	if len(m.agentOrder) < 2 {
		return ""
	}

	leftName := m.agentOrder[0]
	rightName := m.agentOrder[1]
	leftPane := m.agentPanes[leftName]
	rightPane := m.agentPanes[rightName]

	halfWidth := m.width / 2
	rightWidth := m.width - halfWidth

	left := renderAgentColumn(m, leftPane, halfWidth-1, leftName == m.focusedAgent)
	right := renderAgentColumn(m, rightPane, rightWidth-1, rightName == m.focusedAgent)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

// renderStackedAgentPanes renders N agent panes stacked vertically.
func renderStackedAgentPanes(m TUIModel) string {
	var rows []string
	for _, name := range m.agentOrder {
		p := m.agentPanes[name]
		row := renderAgentRow(m, p, m.width, name == m.focusedAgent)
		rows = append(rows, row)
	}
	return strings.Join(rows, "\n")
}

// renderAgentColumn renders one agent's content in a vertical column.
func renderAgentColumn(m TUIModel, p *agentPaneState, width int, focused bool) string {
	if p == nil {
		return ""
	}

	borderStyle := agentUnfocusedStyle
	if focused {
		borderStyle = agentFocusedStyle
	}

	header := renderAgentPaneHeader(p)
	toolsContent := renderToolCallsPanelForPane(m, p, width-4, focused && m.focusedPane == "tools")
	textContent := renderTextPanelForPane(p, width-4, focused && m.focusedPane == "text")

	inner := strings.Join([]string{header, toolsContent, textContent}, "\n")
	return borderStyle.Width(width - 2).Render(inner)
}

// renderAgentRow renders one agent's content in a compact horizontal row (for 3+ agents).
func renderAgentRow(m TUIModel, p *agentPaneState, width int, focused bool) string {
	if p == nil {
		return ""
	}

	borderStyle := agentUnfocusedStyle
	if focused {
		borderStyle = agentFocusedStyle
	}

	header := renderAgentPaneHeader(p)
	// For stacked layout, show tool calls and output side by side
	toolsWidth := width/2 - 4
	if toolsWidth < 20 {
		toolsWidth = 20
	}
	textWidth := width - toolsWidth - 4

	toolsContent := renderToolCallsPanelForPane(m, p, toolsWidth, focused && m.focusedPane == "tools")
	textContent := renderTextPanelForPane(p, textWidth, focused && m.focusedPane == "text")

	inner := header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, toolsContent, " ", textContent)
	return borderStyle.Width(width - 2).Render(inner)
}

// renderAgentPaneHeader renders the agent name, current task, and stats.
func renderAgentPaneHeader(p *agentPaneState) string {
	name := lipgloss.NewStyle().Bold(true).Render(p.agentName)

	phase := p.phase
	switch phase {
	case "running":
		phase = "▶"
	case "polling":
		phase = "◎"
	case "idle":
		phase = "○"
	case "stopped":
		phase = "■"
	default:
		phase = "…"
	}

	stats := ""
	if p.stats.inputTokens > 0 {
		stats = fmt.Sprintf(" ↑%s ↓%s", formatTokens(p.stats.inputTokens), formatTokens(p.stats.outputTokens))
	}
	if p.stats.turns > 0 {
		stats += fmt.Sprintf(" T:%d", p.stats.turns)
	}

	cardInfo := "(idle)"
	if p.activeCard != nil {
		elapsed := time.Since(p.activeCard.started).Round(time.Second)
		cardInfo = fmt.Sprintf("%q %s", truncate(p.activeCard.name, 30), elapsed)
	}

	return fmt.Sprintf("%s %s %s%s", phase, name, cardInfo, stats)
}

// renderToolCallsPanelForPane renders the tool calls panel for a specific pane.
func renderToolCallsPanelForPane(m TUIModel, p *agentPaneState, width int, focused bool) string {
	focusColor := lipgloss.Color("240")
	if focused {
		focusColor = lipgloss.Color("12")
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusColor).
		Width(width - 2).
		Padding(0, 1)

	if len(p.toolCalls) == 0 && p.activeCall == nil {
		return style.Render("Tools\n  (none)")
	}
	return style.Render("Tools\n" + p.toolViewport.View())
}

// renderTextPanelForPane renders the text output panel for a specific pane.
func renderTextPanelForPane(p *agentPaneState, width int, focused bool) string {
	focusColor := lipgloss.Color("240")
	if focused {
		focusColor = lipgloss.Color("12")
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(focusColor).
		Width(width - 2).
		Padding(0, 1)

	label := agentOutputLabel(p.agentName)
	if len(p.textLines) == 0 {
		return style.Render(label + "\n  (no output yet)")
	}
	return style.Render(label + "\n" + p.textViewport.View())
}

// renderFilesPanelForPane renders the files panel for a specific pane.
func renderFilesPanelForPane(p *agentPaneState, width int) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(width - 2).
		Padding(0, 1)

	if len(p.filesRead) == 0 && len(p.filesEdited) == 0 {
		return style.Render("Files\n  (none)")
	}

	var lines []string
	lines = append(lines, "Files")
	for _, f := range p.filesEdited {
		lines = append(lines, "  E "+shortenPath(f))
	}
	for _, f := range p.filesRead {
		lines = append(lines, "  R "+shortenPath(f))
	}
	return style.Render(strings.Join(lines, "\n"))
}

// --- Footer ---

func renderFooter(m TUIModel) string {
	var parts []string

	if m.lastErr != "" {
		parts = append(parts, errorStyle.Render("Error: "+m.lastErr))
	}

	historyStart := 0
	if len(m.history) > 5 {
		historyStart = len(m.history) - 5
	}
	var historyParts []string
	for _, h := range m.history[historyStart:] {
		icon := doneIcon
		if h.status == "failed" {
			icon = failedIcon
		}
		agentTag := ""
		if len(m.agentOrder) > 1 && h.agent != "" {
			agentTag = "[" + h.agent + "] "
		}
		historyParts = append(historyParts, fmt.Sprintf("%s %s%q (%s)", icon, agentTag, h.name, h.duration))
	}

	if len(historyParts) > 0 {
		parts = append(parts, "History: "+strings.Join(historyParts, " | "))
	}

	if len(m.agentOrder) > 1 {
		parts = append(parts, fmt.Sprintf("[Tab: switch agent] [1-%d: select agent] [q: quit]", len(m.agentOrder)))
	} else {
		parts = append(parts, "[Tab: switch pane] [q: quit]")
	}

	return strings.Join(parts, "\n")
}

// renderToolCallsList is kept for compatibility — delegates to renderToolCallsListForPane.
func renderToolCallsList(m TUIModel) string {
	if len(m.agentOrder) == 0 {
		return ""
	}
	p := m.agentPanes[m.agentOrder[0]]
	if p == nil {
		return ""
	}
	return renderToolCallsListForPane(p)
}

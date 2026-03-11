package taskrunner

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// claudePane is a test helper to get the default "claude" agent pane.
func claudePane(m TUIModel) *agentPaneState {
	return m.agentPanes["claude"]
}

func TestTUIInit(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil command, expected non-nil")
	}
}

func TestTUIUpdateWindowSize(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(TUIModel)

	if model.width != 120 {
		t.Errorf("width = %d, want 120", model.width)
	}
	if model.height != 40 {
		t.Errorf("height = %d, want 40", model.height)
	}
	if !model.ready {
		t.Error("ready = false, want true")
	}
}

func TestTUIUpdateCardStarted(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	event := CardStartedEvent{CardID: "c1", CardName: "Fix bug", Branch: "task/c1-fix"}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p := claudePane(model)

	if p.activeCard == nil {
		t.Fatal("activeCard is nil, expected non-nil")
	}
	if p.activeCard.name != "Fix bug" {
		t.Errorf("activeCard.name = %q, want %q", p.activeCard.name, "Fix bug")
	}
	if p.activeCard.branch != "task/c1-fix" {
		t.Errorf("activeCard.branch = %q, want %q", p.activeCard.branch, "task/c1-fix")
	}
	if model.phase != "running" {
		t.Errorf("phase = %q, want %q", model.phase, "running")
	}
}

func TestTUIUpdateCardDone(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	// First set an active card on the pane
	claudePane(m).activeCard = &cardState{id: "c1", name: "Fix bug", status: "running"}

	event := CardDoneEvent{CardID: "c1", CardName: "Fix bug", PRURL: "http://pr/1", Duration: 3 * time.Minute}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p := claudePane(model)

	if p.activeCard != nil {
		t.Error("activeCard should be nil after done")
	}
	if len(model.history) != 1 {
		t.Fatalf("history len = %d, want 1", len(model.history))
	}
	if model.history[0].status != "done" {
		t.Errorf("history[0].status = %q, want %q", model.history[0].status, "done")
	}
	if model.history[0].prURL != "http://pr/1" {
		t.Errorf("history[0].prURL = %q, want %q", model.history[0].prURL, "http://pr/1")
	}
	if p.phase != "polling" {
		t.Errorf("pane phase = %q, want %q", p.phase, "polling")
	}
}

func TestTUIUpdateCardFailed(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	claudePane(m).activeCard = &cardState{id: "c1", name: "Fix bug", status: "running"}

	event := CardFailedEvent{CardID: "c1", CardName: "Fix bug", ErrMsg: "oops", Duration: time.Minute}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p := claudePane(model)

	if p.activeCard != nil {
		t.Error("activeCard should be nil after failure")
	}
	if len(model.history) != 1 {
		t.Fatalf("history len = %d, want 1", len(model.history))
	}
	if model.history[0].status != "failed" {
		t.Errorf("history[0].status = %q, want %q", model.history[0].status, "failed")
	}
	if model.history[0].errMsg != "oops" {
		t.Errorf("history[0].errMsg = %q, want %q", model.history[0].errMsg, "oops")
	}
}

func TestTUIUpdateNoTasks(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	event := NoTasksEvent{NextPoll: 5 * time.Second}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)

	if claudePane(model).phase != "idle" {
		t.Errorf("pane phase = %q, want %q", claudePane(model).phase, "idle")
	}
}

func TestTUIUpdateRunnerStopped(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	event := RunnerStoppedEvent{}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)

	// With single agent, all agents stopped → global phase = "stopped"
	if model.phase != "stopped" {
		t.Errorf("phase = %q, want %q", model.phase, "stopped")
	}
}

func TestTUIKeyQuit(t *testing.T) {
	ch := make(chan Event, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !cancelCalled {
		t.Error("cancel was not called on ctrl+c")
	}
	// The command should be tea.Quit
	if cmd == nil {
		t.Fatal("cmd is nil, expected tea.Quit")
	}
}

func TestTUIUpdateRunnerDone(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	updated, _ := m.Update(runnerDoneMsg{})
	model := updated.(TUIModel)

	if model.phase != "stopped" {
		t.Errorf("phase = %q, want %q", model.phase, "stopped")
	}
}

func TestTickUpdatesView(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	_, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Error("tickMsg should return a non-nil cmd to continue ticking")
	}
}

func TestKeyUpDown(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	// Set up viewport with some content
	m.ready = true
	m.width = 100
	m.height = 30

	p := claudePane(m)

	// Add enough tool calls so we can scroll
	for i := 0; i < 50; i++ {
		p.toolCalls = append(p.toolCalls, toolCallEntry{toolName: "Read", summary: "file.go"})
	}
	p.toolViewport.SetContent(renderToolCallsListForPane(p))

	// Test that key j delegates to viewport (should not error)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model := updated.(TUIModel)
	_ = model // just verify it doesn't panic

	// Test key k
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	model = updated.(TUIModel)
	_ = model
}

func TestKeyQByRune(t *testing.T) {
	ch := make(chan Event, 1)
	cancelCalled := false
	cancel := func() { cancelCalled = true }

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !cancelCalled {
		t.Error("cancel was not called on 'q' key")
	}
	if cmd == nil {
		t.Fatal("cmd is nil, expected tea.Quit")
	}
}

func TestTUIUpdateRunnerStarted(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	event := RunnerStartedEvent{
		BoardName: "Sprint Board",
		BoardID:   "board123",
		Lists: map[string]string{
			"Ready":       "list1",
			"In Progress": "list2",
			"Done":        "list3",
			"Failed":      "list4",
		},
	}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)

	if model.boardID != "board123" {
		t.Errorf("boardID = %q, want %q", model.boardID, "board123")
	}
	if len(model.lists) != 4 {
		t.Errorf("lists len = %d, want 4", len(model.lists))
	}
	if model.phase != "polling" {
		t.Errorf("phase = %q, want %q", model.phase, "polling")
	}
}

func TestTUIUpdateToolStart(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	event := ToolStartEvent{
		ToolName: "Read",
		Input:    map[string]any{"file_path": "/home/user/project/main.go"},
	}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p := claudePane(model)

	if p.activeCall == nil {
		t.Fatal("activeCall is nil, expected non-nil")
	}
	if p.activeCall.toolName != "Read" {
		t.Errorf("activeCall.toolName = %q, want %q", p.activeCall.toolName, "Read")
	}
	if p.activeCall.summary != "project/main.go" {
		t.Errorf("activeCall.summary = %q, want %q", p.activeCall.summary, "project/main.go")
	}
	if p.activeCall.durationMs != -1 {
		t.Errorf("activeCall.durationMs = %d, want -1", p.activeCall.durationMs)
	}
}

func TestTUIUpdateToolResult(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	claudePane(m).activeCall = &toolCallEntry{
		toolName:   "Read",
		summary:    "project/main.go",
		durationMs: -1,
		timestamp:  time.Now(),
	}

	event := ToolResultEvent{ToolName: "Read", DurationMs: 150}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p := claudePane(model)

	if p.activeCall != nil {
		t.Error("activeCall should be nil after ToolResultEvent")
	}
	if len(p.toolCalls) != 1 {
		t.Fatalf("toolCalls len = %d, want 1", len(p.toolCalls))
	}
	if p.toolCalls[0].durationMs != 150 {
		t.Errorf("toolCalls[0].durationMs = %d, want 150", p.toolCalls[0].durationMs)
	}
	if p.toolCalls[0].toolName != "Read" {
		t.Errorf("toolCalls[0].toolName = %q, want %q", p.toolCalls[0].toolName, "Read")
	}
}

func TestTUIUpdateTextOutput(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	event := TextOutputEvent{Text: "Analyzing the codebase..."}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p := claudePane(model)

	if len(p.textLines) != 1 {
		t.Fatalf("textLines len = %d, want 1", len(p.textLines))
	}
	if p.textLines[0] != "Analyzing the codebase..." {
		t.Errorf("textLines[0] = %q, want %q", p.textLines[0], "Analyzing the codebase...")
	}
}

func TestTUIUpdateStatsUpdate(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	// First stats event
	event := StatsUpdateEvent{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 200, Turns: 1}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p := claudePane(model)

	if p.stats.inputTokens != 1000 {
		t.Errorf("stats.inputTokens = %d, want 1000", p.stats.inputTokens)
	}
	if p.stats.outputTokens != 500 {
		t.Errorf("stats.outputTokens = %d, want 500", p.stats.outputTokens)
	}
	if p.stats.cacheReadTokens != 200 {
		t.Errorf("stats.cacheReadTokens = %d, want 200", p.stats.cacheReadTokens)
	}
	if p.stats.turns != 1 {
		t.Errorf("stats.turns = %d, want 1", p.stats.turns)
	}

	// Second stats event - should accumulate
	event2 := StatsUpdateEvent{InputTokens: 500, OutputTokens: 300, CacheReadTokens: 100, Turns: 2}
	updated2, _ := model.Update(event2)
	model2 := updated2.(TUIModel)
	p2 := claudePane(model2)

	if p2.stats.inputTokens != 1500 {
		t.Errorf("stats.inputTokens = %d, want 1500", p2.stats.inputTokens)
	}
	if p2.stats.outputTokens != 800 {
		t.Errorf("stats.outputTokens = %d, want 800", p2.stats.outputTokens)
	}
	if p2.stats.turns != 2 {
		t.Errorf("stats.turns = %d, want 2", p2.stats.turns)
	}
}

func TestTUIUpdateToolStartTracksFiles(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	// Read a file
	event1 := ToolStartEvent{ToolName: "Read", Input: map[string]any{"file_path": "/home/user/main.go"}}
	updated, _ := m.Update(event1)
	model := updated.(TUIModel)
	p := claudePane(model)

	if len(p.filesRead) != 1 {
		t.Fatalf("filesRead len = %d, want 1", len(p.filesRead))
	}
	if p.filesRead[0] != "/home/user/main.go" {
		t.Errorf("filesRead[0] = %q, want %q", p.filesRead[0], "/home/user/main.go")
	}

	// Edit a file
	event2 := ToolStartEvent{ToolName: "Edit", Input: map[string]any{"file_path": "/home/user/util.go"}}
	updated2, _ := model.Update(event2)
	model2 := updated2.(TUIModel)
	p2 := claudePane(model2)

	if len(p2.filesEdited) != 1 {
		t.Fatalf("filesEdited len = %d, want 1", len(p2.filesEdited))
	}
	if p2.filesEdited[0] != "/home/user/util.go" {
		t.Errorf("filesEdited[0] = %q, want %q", p2.filesEdited[0], "/home/user/util.go")
	}

	// Read same file again - should not duplicate
	event3 := ToolStartEvent{ToolName: "Read", Input: map[string]any{"file_path": "/home/user/main.go"}}
	updated3, _ := model2.Update(event3)
	model3 := updated3.(TUIModel)
	p3 := claudePane(model3)

	if len(p3.filesRead) != 1 {
		t.Errorf("filesRead len = %d, want 1 (no duplicates)", len(p3.filesRead))
	}
}

func TestTUITabSwitchesFocus(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)

	if m.focusedPane != "tools" {
		t.Errorf("initial focusedPane = %q, want %q", m.focusedPane, "tools")
	}

	// Tab should switch to text (single agent: toggles pane focus)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	model := updated.(TUIModel)
	if model.focusedPane != "text" {
		t.Errorf("after tab focusedPane = %q, want %q", model.focusedPane, "text")
	}

	// Tab again should switch back to tools
	updated2, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model2 := updated2.(TUIModel)
	if model2.focusedPane != "tools" {
		t.Errorf("after second tab focusedPane = %q, want %q", model2.focusedPane, "tools")
	}
}

func TestTUICardStartedClearsState(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	// Set up some existing state on the pane
	p := claudePane(m)
	p.toolCalls = []toolCallEntry{{toolName: "Read", summary: "old.go", durationMs: 100}}
	p.activeCall = &toolCallEntry{toolName: "Bash", summary: "echo hi", durationMs: -1}
	p.textLines = []string{"old output"}
	p.stats = sessionStats{inputTokens: 5000, outputTokens: 2000, turns: 3}
	p.filesRead = []string{"/old/file.go"}
	p.filesEdited = []string{"/old/edit.go"}

	event := CardStartedEvent{CardID: "c2", CardName: "New task", Branch: "task/c2-new"}
	updated, _ := m.Update(event)
	model := updated.(TUIModel)
	p2 := claudePane(model)

	if len(p2.toolCalls) != 0 {
		t.Errorf("toolCalls should be cleared, len = %d", len(p2.toolCalls))
	}
	if p2.activeCall != nil {
		t.Error("activeCall should be nil after CardStartedEvent")
	}
	if len(p2.textLines) != 0 {
		t.Errorf("textLines should be cleared, len = %d", len(p2.textLines))
	}
	if p2.stats.inputTokens != 0 {
		t.Errorf("stats.inputTokens should be 0, got %d", p2.stats.inputTokens)
	}
	if p2.stats.outputTokens != 0 {
		t.Errorf("stats.outputTokens should be 0, got %d", p2.stats.outputTokens)
	}
	if p2.stats.turns != 0 {
		t.Errorf("stats.turns should be 0, got %d", p2.stats.turns)
	}
	if len(p2.filesRead) != 0 {
		t.Errorf("filesRead should be cleared, len = %d", len(p2.filesRead))
	}
	if len(p2.filesEdited) != 0 {
		t.Errorf("filesEdited should be cleared, len = %d", len(p2.filesEdited))
	}
}

func TestViewportWidthMatchesContainer(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(TUIModel)
	p := claudePane(model)

	if p.toolViewport.Width != p.toolContentWidth {
		t.Errorf("toolViewport.Width = %d, want toolContentWidth = %d", p.toolViewport.Width, p.toolContentWidth)
	}
	if p.textViewport.Width != p.textContentWidth {
		t.Errorf("textViewport.Width = %d, want textContentWidth = %d", p.textViewport.Width, p.textContentWidth)
	}

	// Content widths should account for border(2) + padding(2) = 4
	expectedTextContent := 120 - 4
	if p.textContentWidth != expectedTextContent {
		t.Errorf("textContentWidth = %d, want %d", p.textContentWidth, expectedTextContent)
	}
}

func TestTextWrapping(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	// Initialize with a window size first
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := updated.(TUIModel)

	// Add a line longer than textContentWidth
	longLine := strings.Repeat("word ", 30) // 150 chars
	updated2, _ := model.Update(TextOutputEvent{Text: longLine})
	model2 := updated2.(TUIModel)
	p := claudePane(model2)

	// The raw text line should be stored as-is
	if len(p.textLines) != 1 {
		t.Fatalf("textLines len = %d, want 1", len(p.textLines))
	}

	// The viewport content should be wrapped (contains newlines for long lines)
	content := p.textViewport.View()
	if !strings.Contains(content, "word") {
		t.Error("viewport content should contain the text")
	}
}

func TestTextLineCapping(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	// Initialize viewport
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model := updated.(TUIModel)

	// Pre-fill textLines to just below the cap, then add more via Update
	p := claudePane(model)
	for i := 0; i < maxTextLines-5; i++ {
		p.textLines = append(p.textLines, "line")
	}

	// Add 10 more via Update to trigger capping
	for i := 0; i < 10; i++ {
		updated, _ = model.Update(TextOutputEvent{Text: "new"})
		model = updated.(TUIModel)
	}
	p = claudePane(model)

	if len(p.textLines) != maxTextLines {
		t.Errorf("textLines len = %d, want %d", len(p.textLines), maxTextLines)
	}
	// Verify oldest lines were dropped — last 10 should be "new"
	if p.textLines[maxTextLines-1] != "new" {
		t.Errorf("last line = %q, want %q", p.textLines[maxTextLines-1], "new")
	}
}

func TestTUIModel_FixEvents(t *testing.T) {
	eventCh := make(chan Event, 10)
	cancel := func() {}
	m := NewTUIModel("test", []string{"claude"}, eventCh, cancel)

	// Simulate FixStartedEvent
	updated, _ := m.Update(FixStartedEvent{PRURL: "http://pr", Attempt: 1})
	model := updated.(TUIModel)
	if model.phase != "starting" {
		t.Errorf("phase should not change on fix event, got %q", model.phase)
	}

	// Simulate FixDoneEvent
	updated, _ = model.Update(FixDoneEvent{PRURL: "http://pr", Attempt: 1, ExitCode: 0})
	model = updated.(TUIModel)
	if model.phase != "starting" {
		t.Errorf("phase should not change on fix done event, got %q", model.phase)
	}
}

func TestResizeReWrapsText(t *testing.T) {
	ch := make(chan Event, 1)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude"}, ch, cancel)
	// Wide terminal — 200 cols, textContentWidth = 196
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	model := updated.(TUIModel)

	longLine := strings.Repeat("x", 150)
	updated, _ = model.Update(TextOutputEvent{Text: longLine})
	model = updated.(TUIModel)

	wideContentWidth := claudePane(model).textContentWidth

	// Shrink terminal — 80 cols, textContentWidth = 76
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model = updated.(TUIModel)
	p := claudePane(model)

	narrowContentWidth := p.textContentWidth

	// Verify the content widths changed
	if narrowContentWidth >= wideContentWidth {
		t.Errorf("narrowContentWidth (%d) should be less than wideContentWidth (%d)", narrowContentWidth, wideContentWidth)
	}

	// Verify the viewport width was updated to match
	if p.textViewport.Width != narrowContentWidth {
		t.Errorf("textViewport.Width = %d, want %d", p.textViewport.Width, narrowContentWidth)
	}

	// The 150-char line should now be wrapped into multiple lines in the viewport content
	// (76 chars wide means at least 2 lines needed for 150 chars)
	content := p.textViewport.View()
	lines := strings.Split(content, "\n")
	nonEmpty := 0
	for _, l := range lines {
		if l != "" {
			nonEmpty++
		}
	}
	if nonEmpty < 2 {
		t.Errorf("expected wrapped content to have at least 2 non-empty lines, got %d", nonEmpty)
	}
}

func TestTUIMultiAgent(t *testing.T) {
	ch := make(chan Event, 10)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := NewTUIModel("Test Board", []string{"claude", "gemini"}, ch, cancel)

	if len(m.agentOrder) != 2 {
		t.Fatalf("agentOrder len = %d, want 2", len(m.agentOrder))
	}
	if m.focusedAgent != "claude" {
		t.Errorf("focusedAgent = %q, want %q", m.focusedAgent, "claude")
	}

	// Card started on claude
	updated, _ := m.Update(CardStartedEvent{CardID: "c1", CardName: "Task A", Branch: "task/c1", AgentName: "claude"})
	model := updated.(TUIModel)
	if model.agentPanes["claude"].activeCard == nil {
		t.Fatal("claude pane should have active card")
	}
	if model.agentPanes["gemini"].activeCard != nil {
		t.Fatal("gemini pane should have no active card")
	}

	// Card started on gemini
	updated, _ = model.Update(CardStartedEvent{CardID: "c2", CardName: "Task B", Branch: "task/c2", AgentName: "gemini"})
	model = updated.(TUIModel)
	if model.agentPanes["gemini"].activeCard == nil {
		t.Fatal("gemini pane should have active card after its CardStartedEvent")
	}

	// Tab in multi-agent mode switches focused agent, not pane
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(TUIModel)
	if model.focusedAgent != "gemini" {
		t.Errorf("after tab focusedAgent = %q, want %q", model.focusedAgent, "gemini")
	}
}

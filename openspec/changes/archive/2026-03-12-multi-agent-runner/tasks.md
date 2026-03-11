## 1. Foundation & Agent Abstraction

- [x] 1.1 Create `internal/taskrunner/agents/adapter.go` with AgentAdapter interface
- [x] 1.2 Move existing executor.go logic into `internal/taskrunner/agents/claude.go` adapter
- [x] 1.3 Update executor.go to accept AgentAdapter instead of hardcoding Claude

## 2. Agent Adapters (First Wave)

- [x] 2.1 Create Gemini adapter at `internal/taskrunner/agents/gemini.go` with `gemini -p --output-format stream-json --yolo`
- [x] 2.2 Create Opencode adapter at `internal/taskrunner/agents/opencode.go` with `opencode run --format json`
- [x] 2.3 Create Cursor adapter at `internal/taskrunner/agents/cursor.go` with `cursor-agent -p --output-format stream-json --force`
- [x] 2.4 Implement `BuildCommand()` and `FormatPrompt()` methods for each adapter

## 3. Output Parsing Bridges

- [x] 3.1 Rename existing eventbridge.go to `internal/taskrunner/agents/claude_bridge.go`
- [x] 3.2 Create `internal/taskrunner/agents/gemini_bridge.go` to parse Gemini's stream-json format
- [x] 3.3 Create `internal/taskrunner/agents/opencode_bridge.go` to handle step_start/text/step_finish format
- [x] 3.4 Create `internal/taskrunner/agents/cursor_bridge.go` to parse Cursor's stream-json format
- [x] 3.5 Add fallback text-only parsing for unknown agents in adapter interface

## 4. Event System Updates

- [x] 4.1 Add `AgentName string` field to CardStartedEvent, CardDoneEvent, CardFailedEvent
- [x] 4.2 Add `AgentName string` field to ToolStartEvent, ToolResultEvent, TextOutputEvent, StatsUpdateEvent
- [x] 4.3 Add `AgentName string` field to RunnerStartedEvent and other lifecycle events
- [x] 4.4 Create new AgentRegisteredEvent type
- [x] 4.5 Update all event emitters in runner.go to include AgentName

## 5. Configuration & Loading

- [x] 5.1 Add `AgentConfig` struct to `internal/project/config.go` with Name and optional Model field
- [x] 5.2 Add `Agents []AgentConfig` field to project Config
- [x] 5.3 Update .devpilot.json loading to parse agents array; default to `["claude"]` if missing
- [x] 5.4 Create agent factory function that instantiates adapters by name
- [x] 5.5 Add startup validation: check that all configured agents are available in PATH

## 6. Multi-Agent Runner Infrastructure

- [x] 6.1 Create `MultiRunner` struct in runner.go that holds N Runner instances
- [x] 6.2 Implement MultiRunner.Run() to spawn goroutines for each agent
- [x] 6.3 Update commands.go to create MultiRunner when multiple agents are configured
- [x] 6.4 Ensure each Runner goroutine independently polls, claims cards, and executes tasks
- [x] 6.5 Implement graceful shutdown: all goroutines receive context cancellation and exit cleanly

## 7. TUI Refactor for Multi-Agent

- [x] 7.1 Update TUIModel to hold `agents map[string]*agentRunState` instead of single activeCard
- [x] 7.2 Refactor TUI Update() to route events to correct agent pane based on AgentName
- [x] 7.3 Implement two-agent side-by-side layout (60% left, 40% right columns)
- [x] 7.4 Implement 3+-agent stacked vertical layout with compact headers
- [x] 7.5 Update renderView() to display N agent panes concurrently
- [x] 7.6 Ensure shared history bar at bottom shows all agents' completed cards
- [x] 7.7 Test Tab key switching between agent panes, j/k scrolling per pane

## 8. OpenSpec Multi-Agent Support

- [x] 8.1 Update buildPrompt() in runner.go to detect OpenSpec changes
- [x] 8.2 For Claude: inject `/opsx:apply <name>` skill invocation (existing path)
- [x] 8.3 For non-Claude agents: read proposal.md and tasks.md, inject raw content into prompt
- [x] 8.4 Handle missing OpenSpec gracefully; fall back to raw plan text

## 9. Testing & Validation

- [x] 9.1 Unit tests for each AgentAdapter implementation (command building, prompt formatting)
- [x] 9.2 Unit tests for each output bridge (JSON parsing, event translation)
- [x] 9.3 Integration test: single agent configuration works identically to previous version
- [ ] 9.4 Integration test: two agents poll same board and claim different cards
- [ ] 9.5 Integration test: agent goroutine cleanup on context cancellation
- [ ] 9.6 Manual test: run with `agents: ["claude"]` and verify single-pane TUI
- [ ] 9.7 Manual test: run with `agents: ["claude", "gemini"]` and verify two-pane TUI layout
- [ ] 9.8 Manual test: run with three agents and verify stacked layout
- [ ] 9.9 Manual test: OpenSpec change with Claude agent (skill invocation)
- [ ] 9.10 Manual test: OpenSpec change with non-Claude agent (content injection)

## 10. Documentation & Polish

- [x] 10.1 Update CLAUDE.md with multi-agent configuration examples
- [x] 10.2 Document agent adapter interface for future agent contributors
- [x] 10.3 Add comments to agent implementations explaining agent-specific args and output formats
- [ ] 10.4 Test error messages for missing agents, invalid config, startup failures
- [x] 10.5 Verify backward compatibility: existing single-agent .devpilot.json still works

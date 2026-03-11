## Why

DevPilot currently locks all task automation to Claude Code as the sole execution agent. As alternative agents (Gemini CLI, Opencode, Cursor Agent) have matured with headless support and stable JSON output, teams want to mix agents—using the best tool for each task type (Claude for complex reasoning, Gemini for quick edits, Opencode for API work). Running multiple agents in parallel against the same Trello board maximizes throughput: while Claude works on a complex task, Gemini can simultaneously process another card.

## What Changes

- **AgentAdapter interface** — abstract the agent execution layer so any headless CLI agent can be plugged in (currently hardcoded to Claude).
- **Multi-agent parallel runner** — replace single-threaded runner with independent goroutines per agent. Trello's "move to In Progress" acts as distributed lock; first agent to claim a card owns it.
- **Multi-agent TUI** — extend Bubble Tea dashboard to display N concurrent agent panes (side-by-side for 2 agents, stacked for 3+) with separate tool-call histories and output streams.
- **OpenSpec support for all agents** — Claude uses `/opsx:apply` skill; non-Claude agents receive raw OpenSpec change content (proposal.md + tasks.md) injected directly as prompt text.
- **Agent configuration in .devpilot.json** — new `agents` array with agent names and optional model overrides. Single agent or missing config defaults to `["claude"]` for backward compatibility.
- **Output parsing per agent** — Claude/Gemini/Cursor emit stream-json; Opencode emits step-based JSON. Event adapters translate each format into unified event types.

## Capabilities

### New Capabilities

- `agent-abstraction`: Pluggable agent adapters so new CLI agents can be added without modifying core runner logic.
- `multi-agent-parallel-execution`: Spawn independent agent goroutines polling the same board simultaneously, each claiming and processing different cards.
- `agent-aware-tui`: Multi-pane TUI dashboard showing concurrent agent activity, tool calls, and output streams.
- `openspec-multi-agent`: Support OpenSpec change execution across all agents (Claude via skill, others via raw content injection).
- `agent-configuration`: Config schema and loading for agent selection in .devpilot.json.

### Modified Capabilities

- `task-runner-core`: Existing single-threaded runner refactored to support agent adapters and parallel multi-agent mode. Backward compatible: single agent behaves identically to current behavior.
- `task-configuration`: Extend `.devpilot.json` to include agents array and optional agent-specific settings (e.g., Gemini model selection).
- `event-system`: Add optional `AgentName` field to all existing events so TUI can route them to correct agent pane.

## Impact

- **Breaking**: None (single-agent default preserves existing workflows).
- **Files modified**: `internal/taskrunner/`, `internal/project/config.go`, TUI rendering.
- **New package**: `internal/taskrunner/agents/` with adapter pattern and per-agent implementations (claude, gemini, opencode, cursor).
- **Dependencies**: None new (Gemini, Opencode, Cursor CLI are user-installed, not project dependencies).
- **Config schema**: .devpilot.json gains `agents: [{ name: "claude", model?: "..." }]`.
- **OpenSpec integration**: No breaking changes; slide in alongside existing `/opsx:apply` path.

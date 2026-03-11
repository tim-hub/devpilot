## Context

DevPilot's task runner (`internal/taskrunner/`) is currently single-threaded, always using Claude Code via `executor.go`'s hardcoded `claude -p` invocation. The event system (`eventbridge.go`) parses Claude's `stream-json` output into structured `Event` types for the TUI.

Supported agents have headless modes:
- **Claude Code**: `claude -p --output-format stream-json` (existing, stable)
- **Gemini CLI**: `gemini -p --output-format stream-json --yolo` (nearly identical interface)
- **Opencode**: `opencode run --format json` (different JSON structure: `step_start`, `text`, `step_finish`)
- **Cursor Agent**: `cursor-agent -p --output-format stream-json --force` (similar to Claude)

## Goals / Non-Goals

**Goals:**
- Support parallel execution: multiple agents processing different cards from the same Trello board simultaneously.
- Pluggable agents: clean abstraction so new CLI agents can be added without modifying core runner code.
- Unified TUI: show all concurrent agents in one dashboard with separate panes and event streams.
- OpenSpec compatibility: ensure OpenSpec change invocation works across all agents.
- Backward compatible: single-agent defaults match current behavior exactly.

**Non-Goals:**
- Support GitHub Copilot CLI (output format not GA yet).
- Multi-agent coordination (agents don't cooperate; each claims different cards).
- Agent fallback / retry (if an agent fails, the card is marked failed; no auto-reassignment).
- Custom agent binaries (agents must be CLIs in PATH; no wrapper scripts).

## Decisions

### 1. AgentAdapter Interface Pattern

**Decision:** Create an `AgentAdapter` interface so each agent encapsulates its invocation, output parsing, and prompt formatting.

```go
type AgentAdapter interface {
    Name() string                                          // "claude", "gemini", etc.
    BuildCommand(prompt string) (cmd string, args []string)
    ParseLine(line []byte) (Event, bool)                  // agent-specific JSON → Event
    FormatPrompt(task Task, useOpenSpec bool) string      // prompt template
}
```

**Rationale:**
- Centralizes agent-specific logic (command args, output format) so runner code stays agent-agnostic.
- New agents can be added by implementing one interface; no changes to runner or executor.
- Each agent adapter owns the translation from its JSON format → unified Event types.

**Alternatives considered:**
- Monolithic switch statement in executor: brittle, violates single responsibility.
- Separate agent CLI wrappers: adds complexity; agents are already CLI tools.
- Shared event format across all agents: impossible (Opencode's step-based model is different from stream-json).

### 2. Multi-Agent Runner Architecture: Independent Goroutines

**Decision:** Replace single-threaded `Runner` with `MultiRunner` spawning N independent `Runner` goroutines (one per agent). Each runner polls the same Trello board; the first runner to move a card to "In Progress" claims it.

```
MultiRunner (main goroutine)
  ├─ AgentRunner("claude") → polls → claims Card A → creates branch task/A-... → executes → PR
  ├─ AgentRunner("gemini") → polls → claims Card B → creates branch task/B-... → executes → PR
  └─ AgentRunner("cursor") → polls → claims Card C → creates branch task/C-... → executes → PR
```

**Rationale:**
- Trello's API is the distributed lock: "move to In Progress" is atomic. Two runners racing for the same card will both attempt the move; only one succeeds. The other polls again immediately and finds the next card.
- No shared state between runners = no mutexes or coordination channels needed.
- Scales horizontally: add more agents just by extending the config array.

**Alternatives considered:**
- Shared task queue with mutex: overcomplicates; Trello IS the queue.
- One runner with agent-switching logic: would need to track which agent should pick next card (complex priority system).

### 3. Event System: AgentName Routing

**Decision:** All existing events get an optional `AgentName string` field. The TUI routes events to the correct agent pane based on `AgentName`.

```go
type CardStartedEvent struct {
    CardID    string
    CardName  string
    Branch    string
    AgentName string  // ← NEW
}

// All other events gain AgentName similarly
```

**Rationale:**
- TUI model becomes `agents map[string]*agentRunState`, keyed by agent name.
- Each pane updates independently; no crosstalk.
- Backward compatible: single agent just has one pane.

**Alternatives considered:**
- Separate event channels per agent: requires TUI to merge channels (more complex).
- Timestamp-based event ordering: fragile if events are slightly out of order; agent name is explicit.

### 4. Output Parsing: Agent-Specific Bridges

**Decision:** Each agent gets a dedicated bridge that translates its output format → unified `Event` types.

- **claude_bridge.go**: parses Claude's stream-json → ToolStartEvent, ToolResultEvent, etc. (existing code, renamed).
- **gemini_bridge.go**: parses Gemini's stream-json (similar format, slight differences) → events.
- **opencode_bridge.go**: parses Opencode's step-based JSON → TextOutputEvent (simpler; no tool granularity).
- **cursor_bridge.go**: parses Cursor's stream-json → events.

**Rationale:**
- Each agent's output format is different enough to warrant its own parser.
- Fallback for unknown formats: emit `TextOutputEvent` for every line (text-only mode).
- TUI gracefully degrades: agents with rich events show tool calls; text-only agents show output stream.

**Trade-off:**
- Opencode produces no tool-call granularity, so TUI shows less detail. Users understand this as a trade-off for using multiple agents.

### 5. TUI Multi-Pane Layout

**Decision:**
- **2 agents**: side-by-side columns (left/right).
- **3+ agents**: stacked vertically with compact headers.
- All agents share the bottom history bar.

```
Two agents:                        Three agents:
┌─────────────┬─────────────┐     ┌────────────────────────┐
│ claude      │ gemini      │     │ claude                 │
│ ▶ Task A    │ ▶ Task B    │     │ ▶ Task A               │
│ [tools]     │ [tools]     │     │ [tools]                │
├─────────────┼─────────────┤     ├────────────────────────┤
│ ✅ Task X   │ ✅ Task Y   │     │ gemini                 │
│ 2m30s       │ 1m45s       │     │ ▶ Task B               │
└─────────────┴─────────────┘     │ [output]               │
                                  ├────────────────────────┤
                                  │ cursor                 │
                                  │ ▶ Task C               │
                                  │ [output]               │
                                  ├────────────────────────┤
                                  │ ✅ Task X ✅ Task Y    │
                                  └────────────────────────┘
```

**Rationale:**
- Two agents is the common case; side-by-side is natural.
- 3+ agents stack to avoid cramped columns.
- History bar is shared because it's summary-level; detail panes are per-agent.

### 6. Backward Compatibility: Single-Agent Default

**Decision:** If `.devpilot.json` lacks `agents` or `agents` is empty, default to `agents: [{ name: "claude" }]`. Single-agent execution path is identical to current behavior.

**Rationale:**
- Existing projects continue working unchanged.
- Multi-agent is additive, not breaking.

### 7. OpenSpec Handling: Dual Paths

**Decision:**
- **Claude**: Uses existing skill system. Runner detects OpenSpec and injects `/opsx:apply <name>` into prompt.
- **Other agents**: No skill system. Runner detects OpenSpec, reads proposal.md + tasks.md, injects raw content directly into prompt.

```
Claude prompt:
  "Run: /opsx:apply my-feature"

Gemini/Opencode/Cursor prompt:
  "Execute the following OpenSpec change:
   [full content of proposal.md]
   [full content of tasks.md]"
```

**Rationale:**
- Claude's `/opsx:apply` is a native skill; most concise and idiomatic.
- Non-Claude agents don't have the skill system, so we fall back to raw content injection.
- All agents get OpenSpec support without requiring agent-level skill implementations.

**Trade-off:**
- Prompt format differs per agent. Agents may interpret the raw content differently than Claude interprets the skill command.

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| **Race condition claiming cards** | Trello API "move to In Progress" is atomic. Two runners attempting simultaneously; only one succeeds. The loser polls again and finds the next card. |
| **Different agents produce different quality/behavior on same task** | Expected; users choose agents strategically. No auto-fallback; if an agent fails, task is marked failed. |
| **Opencode has no tool-call visibility** | TUI degrades gracefully to text-only mode for Opencode. Users understand this trade-off. |
| **Complex TUI rendering with 3+ agents** | Vertical stacking with compact headers keeps layout clean. Max ~5 agents before terminal becomes cramped. |
| **OpenSpec prompt injection may not parse uniformly** | Each non-Claude agent may interpret raw proposal/tasks differently. Users can test and adjust prompts if needed. |
| **Agent availability (not in PATH)** | Runner checks at startup and fails fast if agent binary not found. |
| **Context window conflicts** | Each agent has its own context budget. Complex tasks may timeout on smaller-context agents (e.g., Cursor). |

## Migration Plan

1. **Phase 1**: Implement `AgentAdapter` interface. Move existing Claude code to `claude.go`. Keep single-agent path identical.
2. **Phase 2**: Implement other agent adapters (gemini, opencode, cursor).
3. **Phase 3**: Build `MultiRunner` and multi-goroutine polling logic.
4. **Phase 4**: Extend event system with `AgentName` field.
5. **Phase 5**: Refactor TUI to multi-pane layout.
6. **Phase 6**: OpenSpec integration for all agents.

All phases preserve backward compatibility. Existing single-agent workflows work unchanged until users opt into multi-agent via `.devpilot.json`.

## Open Questions

1. **Agent timeout / failure behavior**: If one agent hangs, does it block others? (Answer: No; each runs in own goroutine. Timeout is per-card, managed by context deadline.)
2. **Agent-specific prompting**: Should prompts be customizable per agent in config? (Answer: Future enhancement; start with uniform prompts across agents.)
3. **Trello board lock**: What if all agents finish before new cards appear? (Answer: They all go idle and poll at configured interval; no special handling needed.)

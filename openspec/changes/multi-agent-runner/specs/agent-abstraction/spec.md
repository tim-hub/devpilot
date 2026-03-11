## ADDED Requirements

### Requirement: AgentAdapter interface defines agent contract
The system SHALL define an `AgentAdapter` interface that abstracts the details of invoking any headless CLI agent. Each agent implementation (Claude, Gemini, Opencode, Cursor) SHALL implement this interface to provide command construction, output parsing, and prompt formatting.

#### Scenario: Agent adapter invocation
- **WHEN** a runner needs to execute a task with a specific agent
- **THEN** the runner calls methods on the agent's `AgentAdapter` implementation to build the command, parse output, and format the prompt

### Requirement: Agent output parsing is agent-specific
The system SHALL provide agent-specific parsers (bridges) that translate each agent's JSON output format into unified `Event` types (ToolStartEvent, ToolResultEvent, TextOutputEvent, etc.).

#### Scenario: Claude output parsing
- **WHEN** Claude Code emits `stream-json` formatted events
- **THEN** the Claude bridge parses them into ToolStartEvent, ToolResultEvent, StatsUpdateEvent

#### Scenario: Opencode output parsing
- **WHEN** Opencode emits step-based JSON (step_start, text, step_finish)
- **THEN** the Opencode bridge translates them into TextOutputEvent

#### Scenario: Fallback to text-only parsing
- **WHEN** an agent's output cannot be fully parsed into structured events
- **THEN** the bridge emits TextOutputEvent for each line (graceful degradation)

### Requirement: New agent support requires only adapter implementation
The system SHALL make it possible to add support for a new CLI agent by implementing the `AgentAdapter` interface and optional output bridge, without modifying core runner or executor code.

#### Scenario: Adding a new agent
- **WHEN** a new headless CLI agent becomes available
- **THEN** an operator can implement `AgentAdapter` and register it in the agent factory; no changes to runner.go or executor.go needed

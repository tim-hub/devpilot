# Multi-Agent Parallel Execution Capability

## ADDED Requirements

### Requirement: Multiple Agents Configuration
The system SHALL support running multiple AI agents in parallel, configured via `.devpilot.yaml`.

#### Scenario: Single agent (default)
- **WHEN** `.devpilot.yaml` has no `agents` key or `agents` is an empty list
- **THEN** the system defaults to running a single Claude agent (backward compatible)

#### Scenario: Multiple agents configured
- **WHEN** `.devpilot.yaml` contains `agents: [claude, gemini, opencode]`
- **THEN** the system spawns three independent Runner goroutines, one per agent

### Requirement: Parallel Task Claiming with Distributed Lock
The system SHALL run multiple agents concurrently, with each agent independently claiming and executing tasks using the distributed locking mechanism.

#### Scenario: Two agents, one task
- **WHEN** two agents are running and the "Ready" list has one card
- **THEN** both agents poll concurrently; one agent successfully claims the card (via "Claimed By" field) and executes it; the other agent detects the claim collision and moves to the next card

#### Scenario: Two agents, two tasks
- **WHEN** two agents are running and the "Ready" list has two cards
- **THEN** each agent claims a different card; both execute simultaneously

#### Scenario: Three agents, one task
- **WHEN** three agents are running and the "Ready" list has one card
- **THEN** all three agents poll concurrently; one agent claims the card; the other two detect collisions and skip

### Requirement: Independent Execution Pipelines
Each agent SHALL execute tasks independently without shared state between agent goroutines.

#### Scenario: Agent A creates branch and PR while agent B works on different card
- **WHEN** agent A is executing card X (branch creation, code execution, PR) and agent B is executing card Y
- **THEN** the two executions run in parallel without waiting for each other; git operations on different branches do not interfere

#### Scenario: Agent crash isolation
- **WHEN** one agent crashes or receives a timeout
- **THEN** the other agents continue polling and executing unaffected

### Requirement: Multi-Agent TUI Dashboard
The system SHALL adapt the TUI to display multiple agents' progress when multiple agents are running.

#### Scenario: Single-agent TUI
- **WHEN** one agent is running
- **THEN** the dashboard shows the original single-pane layout (backward compatible)

#### Scenario: Two-agent TUI
- **WHEN** two agents are running
- **THEN** the dashboard displays side-by-side columns, one per agent, showing each agent's current task, tool calls, and output

#### Scenario: Three-plus-agent TUI
- **WHEN** three or more agents are running
- **THEN** the dashboard displays stacked vertical rows, one per agent, with compact headers and shared footer

### Requirement: Multi-Agent Event Routing
The system SHALL route events to the correct agent's TUI pane based on the agent name.

#### Scenario: Event tagged with agent name
- **WHEN** agent "claude" emits a `ToolStartEvent` with `AgentName: "claude"`
- **THEN** the TUI routes this event to Claude's pane and updates its tool call list

#### Scenario: Separate stats per agent
- **WHEN** agents run concurrently and both emit `StatsUpdateEvent`
- **THEN** the dashboard aggregates stats per agent and displays cumulative totals in the header

### Requirement: Graceful Shutdown
The system SHALL cleanly shut down all agent goroutines when the runner receives a cancel signal.

#### Scenario: User presses Ctrl-C
- **WHEN** the user presses Ctrl-C or sends SIGINT to `devpilot run`
- **THEN** the runner cancels the context; all agent goroutines receive the cancellation and exit cleanly; any in-flight operations are aborted

#### Scenario: Task timeout
- **WHEN** an agent's task exceeds the configured timeout
- **THEN** the agent's context times out; that agent aborts and returns to polling; other agents continue unaffected

### Requirement: OpenSpec Multi-Agent Support
The system SHALL support OpenSpec changes with multiple agents using agent-appropriate mechanisms.

#### Scenario: Claude with OpenSpec
- **WHEN** a Claude agent runs a task backed by an OpenSpec change
- **THEN** the agent receives `/opsx:apply <change-name>` in its prompt (using the built-in Claude Code skill)

#### Scenario: Non-Claude agent with OpenSpec
- **WHEN** a Gemini or other non-Claude agent runs a task backed by an OpenSpec change
- **THEN** the agent receives the raw `proposal.md` and `tasks.md` content injected directly into the prompt (no skill invocation)

#### Scenario: Mixed agents with OpenSpec
- **WHEN** Claude and Gemini agents both execute tasks from the same OpenSpec change
- **THEN** Claude uses the skill; Gemini receives raw content; both support resumability from last unchecked task

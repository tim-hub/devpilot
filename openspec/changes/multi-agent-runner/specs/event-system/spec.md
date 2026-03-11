## ADDED Requirements

### Requirement: All events carry optional AgentName field
The system SHALL add an `AgentName string` field to all runner event types, allowing the TUI and event handlers to route events to the correct agent context.

#### Scenario: Tool events include agent name
- **WHEN** ToolStartEvent is emitted
- **THEN** it includes AgentName="claude" (or the agent's name)

#### Scenario: Card lifecycle events include agent name
- **WHEN** CardStartedEvent, CardDoneEvent, or CardFailedEvent is emitted
- **THEN** each includes AgentName identifying which agent is processing the card

### Requirement: Single-agent events maintain backward compatibility
The system SHALL ensure that single-agent workflows (one agent) work identically to the previous implementation, with AgentName automatically set and events routed transparently.

#### Scenario: Single Claude agent
- **WHEN** runner is configured with one agent (Claude)
- **THEN** all events are emitted with AgentName="claude"; TUI receives and processes them as before (single pane)

### Requirement: Multi-agent events are independently routed
The system SHALL route incoming events to separate TUI panes based on AgentName, ensuring concurrent agent activity does not interfere.

#### Scenario: Concurrent events from different agents
- **WHEN** Agent 1 emits TextOutputEvent and Agent 2 emits ToolStartEvent simultaneously
- **THEN** TextOutputEvent is routed to Agent 1 pane, ToolStartEvent to Agent 2 pane; both update independently

### Requirement: Event stream merging is deterministic
The system SHALL merge event streams from multiple agents into a single ordered sequence without loss or reordering within each agent's stream.

#### Scenario: Event ordering from multiple agents
- **WHEN** three agents are emitting events to the same channel
- **THEN** all events are delivered in order, preserving the sequence within each agent's stream

### Requirement: New event type: AgentRegisteredEvent
The system SHALL emit an AgentRegisteredEvent at startup for each configured agent, announcing its availability and name.

#### Scenario: Agent startup announcement
- **WHEN** runner starts with `agents: [{ "name": "claude" }, { "name": "gemini" }]`
- **THEN** AgentRegisteredEvent is emitted for each agent, identifying them to the TUI

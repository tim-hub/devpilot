## ADDED Requirements

### Requirement: TUI displays concurrent agent activity
The system SHALL display N agent panes in the Bubble Tea TUI, each showing the active task, tool calls, text output, and stats for that agent.

#### Scenario: Two-agent side-by-side layout
- **WHEN** two agents are running
- **THEN** the TUI displays two columns: left pane for Agent 1, right pane for Agent 2

#### Scenario: Three-or-more-agent stacked layout
- **WHEN** three or more agents are running
- **THEN** the TUI displays agents in vertical stacked panes, each with a compact header

### Requirement: Events are routed to correct agent pane
The system SHALL include an `AgentName` field in all runner events. The TUI model SHALL route incoming events to the pane corresponding to the event's agent name.

#### Scenario: Tool call in Agent 1 pane
- **WHEN** Agent 1 executes a tool call
- **THEN** a ToolStartEvent with AgentName="claude" is emitted and routed to the Claude pane; the Gemini pane is unaffected

### Requirement: Each agent pane tracks independent state
The system SHALL maintain separate state (activeCard, toolCalls, textLines, stats, filesRead, filesEdited) for each agent pane. State changes in one pane do not affect others.

#### Scenario: Concurrent tool calls on different agents
- **WHEN** Agent 1 is reading a file while Agent 2 is executing a command
- **THEN** the file read appears in Agent 1's tool history; the command appears in Agent 2's tool history

### Requirement: Shared history bar at bottom
The system SHALL display a shared history bar at the bottom of the TUI showing completed cards across all agents (status icons, card names, durations, PR links).

#### Scenario: History shows all agents' completed cards
- **WHEN** Agent 1 completes Card A and Agent 2 completes Card B
- **THEN** both completed cards appear in the shared history bar with their agent names or icons

### Requirement: Agent pane focus and scroll
The system SHALL allow the user to switch focus between agent panes (Tab key) and scroll within the focused pane (j/k/↑/↓ keys).

#### Scenario: User switches between agent panes
- **WHEN** user presses Tab
- **THEN** focus moves to the next agent pane; scrolling keys affect the focused pane only

## ADDED Requirements

### Requirement: Multi-agent runner supports parallel task execution
The system SHALL refactor the Runner to support multiple agents executing different tasks concurrently. A MultiRunner SHALL spawn independent agent runners, each with its own adapter, polling, and execution pipeline.

#### Scenario: Single agent (backward compatible)
- **WHEN** `agents: [{ "name": "claude" }]` is configured (or missing)
- **THEN** the runner behaves identically to the previous single-agent implementation

#### Scenario: Multiple agents
- **WHEN** `agents: [{ "name": "claude" }, { "name": "gemini" }]` is configured
- **THEN** two independent runners execute concurrently, each claiming and processing different cards

### Requirement: Runner accepts AgentAdapter at initialization
The system SHALL allow Runner to be initialized with a specific AgentAdapter, making the executor agent-agnostic.

#### Scenario: Runner with Claude adapter
- **WHEN** Runner is created with claudeAdapter
- **THEN** it invokes `claude -p --output-format stream-json` for all tasks

#### Scenario: Runner with Gemini adapter
- **WHEN** Runner is created with geminiAdapter
- **THEN** it invokes `gemini -p --output-format stream-json --yolo` for all tasks

### Requirement: Distributed card claiming via Trello board state
The system SHALL use Trello's "move to In Progress" as the atomic claim operation. Multiple runners can safely race for cards; the first to succeed owns the card.

#### Scenario: Race-free card claiming
- **WHEN** two runners simultaneously poll and see three ready cards
- **THEN** each runner independently attempts to move different cards to In Progress. Both succeed (one per card); both continue polling for remaining cards.

### Requirement: Runner event emission includes AgentName
The system SHALL emit events with AgentName set to identify which agent produced each event.

#### Scenario: CardStartedEvent with agent name
- **WHEN** an agent begins processing a card
- **THEN** CardStartedEvent is emitted with AgentName="claude" (or agent's name)

### Requirement: Per-agent logging and error handling
The system SHALL isolate errors and logs per agent so failures in one agent do not affect others.

#### Scenario: Agent A fails, Agent B continues
- **WHEN** Agent A encounters a task execution error
- **THEN** Agent A marks its card as Failed; Agent B polls and processes remaining cards independently

## ADDED Requirements

### Requirement: Multiple agents execute tasks in parallel
The system SHALL support spawning independent goroutines for each configured agent, each simultaneously polling the same Trello board, claiming different cards, and executing tasks.

#### Scenario: Two agents claim different cards
- **WHEN** two agents are configured and three cards are ready
- **THEN** Agent 1 claims Card A, Agent 2 claims Card B, and both execute in parallel. Card C remains in Ready until one agent finishes.

#### Scenario: One agent claims a card before another
- **WHEN** two agents attempt to claim the same card (race condition)
- **THEN** the first agent to move it to "In Progress" succeeds; the second fails atomically and polls for the next available card

### Requirement: Trello board state is the distributed lock
The system SHALL use Trello's atomic "move to In Progress" operation as the lock mechanism. No additional mutex or coordination between agents is needed; all claimed-vs-unclaimed logic is derived from the board state.

#### Scenario: Agent failure does not block others
- **WHEN** one agent encounters an error and marks its card as Failed
- **THEN** other agents continue polling and processing other cards independently

### Requirement: Each agent runs the full task pipeline
The system SHALL ensure that each agent, once it claims a card, executes the entire pipeline: create branch, execute task via agent binary, push PR, code review (if enabled), and move card to Done/Failed.

#### Scenario: Independent task lifecycle
- **WHEN** two agents are processing two different cards
- **THEN** each agent independently manages git branches, commits, PR creation, and Trello state transitions for its card

### Requirement: Agent goroutines are cleanly stoppable
The system SHALL ensure that when the runner is stopped (via context cancellation or signal), all agent goroutines are gracefully shut down.

#### Scenario: Graceful shutdown with multiple agents
- **WHEN** user sends Ctrl-C to the runner
- **THEN** all agent goroutines receive cancellation signal, complete their current task (if any), and exit cleanly within timeout

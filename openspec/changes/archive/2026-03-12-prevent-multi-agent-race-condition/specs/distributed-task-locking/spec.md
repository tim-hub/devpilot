# Distributed Task Locking Capability

## ADDED Requirements

### Requirement: Claim Custom Field Management
The system SHALL automatically create and manage a "Claimed By" Trello custom field on the board to track task ownership across agents.

#### Scenario: Auto-create field on first run
- **WHEN** `devpilot run` starts with multiple agents configured and the board has no "Claimed By" field
- **THEN** the system creates a custom field named "Claimed By" on the board

#### Scenario: Reuse existing field
- **WHEN** `devpilot run` starts with multiple agents and the "Claimed By" field already exists
- **THEN** the system uses the existing field without attempting to recreate it

### Requirement: Task Claiming
The system SHALL set the "Claimed By" field when an agent attempts to claim a task.

#### Scenario: Successful claim
- **WHEN** an agent moves a card to "In Progress"
- **THEN** the agent sets the "Claimed By" field to `{agent-name}:{unix-timestamp-ms}` (e.g., `claude:1710250800123`)

#### Scenario: Claim persists
- **WHEN** an agent has claimed a card
- **THEN** the "Claimed By" value remains set on the card until the agent completes the task

### Requirement: Ownership Verification
The system SHALL verify ownership before executing a claimed task to detect and handle race conditions.

#### Scenario: Verify before branch creation
- **WHEN** an agent has claimed a card and is about to create a git branch
- **THEN** the agent re-fetches the card and verifies the "Claimed By" field matches its own name and timestamp

#### Scenario: Own the card
- **WHEN** verification succeeds (field matches the agent's claim)
- **THEN** the agent proceeds with branch creation and execution

#### Scenario: Lose the card to another agent
- **WHEN** verification fails (field set to a different agent or newer timestamp)
- **THEN** the agent aborts execution, logs a claim collision event, and skips to the next card

### Requirement: Collision Observability
The system SHALL provide visibility into task claiming and collision events.

#### Scenario: Log claim collision
- **WHEN** an agent loses a task to another agent (verification fails)
- **THEN** the system emits a `ClaimCollisionEvent` with the agent name, card ID, card name, and timestamp

#### Scenario: Include claim info in execution logs
- **WHEN** a task is successfully claimed and executed
- **THEN** the execution log includes which agent claimed it and the claim timestamp

### Requirement: Graceful Degradation
The system SHALL work correctly even if the "Claimed By" field is unavailable.

#### Scenario: Single agent mode
- **WHEN** only one agent is configured
- **THEN** the system does not require the "Claimed By" field and functions normally (backward compatible)

#### Scenario: Field creation fails
- **WHEN** the system cannot create or access the "Claimed By" field (e.g., Trello API error)
- **THEN** the system logs a warning and continues; if multiple agents are running, they may have higher collision risk but won't crash

### Requirement: Claim Lifecycle
Tasks claimed by agents must be tracked through completion.

#### Scenario: Clear claim on task completion
- **WHEN** an agent completes a task and moves the card to "Done"
- **THEN** the system clears the "Claimed By" field (or sets it to a marker like "completed")

#### Scenario: Recover from agent crash
- **WHEN** an agent crashes without completing the task and the "Claimed By" field remains set for longer than the configured task timeout
- **THEN** manual intervention or a cleanup process can reset the field for re-claiming by another agent (documented as unsupported auto-recovery in V1)

## ADDED Requirements

### Requirement: Claude uses skill invocation for OpenSpec changes
The system SHALL detect OpenSpec installation and, when processing a task, use the `/opsx:apply <change-name>` skill if the card description is an OpenSpec change name.

#### Scenario: Claude executes OpenSpec change via skill
- **WHEN** openspec is installed AND card description is "my-feature" (matching openspec/changes/my-feature/)
- **THEN** Claude is invoked with prompt "Run: /opsx:apply my-feature"

### Requirement: Non-Claude agents receive OpenSpec content as raw text
The system SHALL detect when a task is an OpenSpec change and, for agents other than Claude, inject the raw proposal.md and tasks.md content directly into the prompt.

#### Scenario: Gemini receives OpenSpec change content
- **WHEN** Openspec is installed AND Gemini agent is processing a card with OpenSpec change name
- **THEN** Gemini's prompt includes the full text of proposal.md and tasks.md from that change

#### Scenario: Opencode receives OpenSpec change content
- **WHEN** Openspec is installed AND Opencode is processing a card with OpenSpec change name
- **THEN** Opencode's prompt includes the full text of proposal.md and tasks.md

### Requirement: OpenSpec content injection is transparent to agents
The system SHALL handle the detection and injection of OpenSpec content at the runner level, not requiring agents to have special OpenSpec awareness.

#### Scenario: Agent receives complete, self-contained prompt
- **WHEN** an agent processes an OpenSpec change
- **THEN** the agent receives a complete prompt with all necessary context; no agent-side OpenSpec integration is required

### Requirement: Non-OpenSpec cards continue to work unchanged
The system SHALL fall back to existing prompt format (raw plan text from card description) if OpenSpec is not detected.

#### Scenario: Claude processes non-OpenSpec card
- **WHEN** card description is freeform task text (not an OpenSpec change name)
- **THEN** Claude receives the existing prompt format with task plan text

#### Scenario: Gemini processes non-OpenSpec card
- **WHEN** card description is freeform task text
- **THEN** Gemini receives the same prompt as if openspec were not installed

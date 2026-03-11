## ADDED Requirements

### Requirement: .devpilot.json supports agents array
The system SHALL add an `agents` field to .devpilot.json that allows users to configure which agents to use and optional agent-specific settings.

#### Scenario: Single agent configuration
- **WHEN** user adds `"agents": [{ "name": "claude" }]` to .devpilot.json
- **THEN** the runner uses only Claude Code for task execution

#### Scenario: Multiple agents configuration
- **WHEN** user adds `"agents": [{ "name": "claude" }, { "name": "gemini" }, { "name": "opencode" }]`
- **THEN** the runner spawns independent goroutines for Claude, Gemini, and Opencode, each polling the board

#### Scenario: Agent with model override
- **WHEN** user adds `{ "name": "gemini", "model": "gemini-2.0" }`
- **THEN** the Gemini adapter uses `--model gemini-2.0` in its command arguments

### Requirement: Default agent is Claude
The system SHALL use Claude as the default agent if the `agents` field is missing or empty in .devpilot.json.

#### Scenario: No agents field configured
- **WHEN** .devpilot.json has no `agents` field
- **THEN** the runner defaults to `agents: [{ "name": "claude" }]`

#### Scenario: Empty agents array
- **WHEN** .devpilot.json has `"agents": []`
- **THEN** the runner defaults to `agents: [{ "name": "claude" }]`

### Requirement: Agent availability is checked at startup
The system SHALL verify that each configured agent's binary is available in PATH at runner startup. Missing agents cause a clear error message.

#### Scenario: Agent binary not found
- **WHEN** user configures `{ "name": "gemini" }` but `gemini` is not installed
- **THEN** runner exits with error: "Agent 'gemini' not found in PATH"

### Requirement: Config is backward compatible
The system SHALL ensure that existing .devpilot.json files without the `agents` field continue to work, defaulting to single-agent Claude mode.

#### Scenario: Legacy config without agents field
- **WHEN** an existing .devpilot.json (without `agents`) is used with the new runner
- **THEN** the runner works identically to the previous version, using Claude only

### Requirement: Agent configuration validation
The system SHALL validate agent names and settings at startup, reporting errors for unknown agent names or invalid model values.

#### Scenario: Unknown agent name
- **WHEN** user configures `{ "name": "unknown-agent" }`
- **THEN** runner exits with error: "Unknown agent 'unknown-agent'" listing available agents

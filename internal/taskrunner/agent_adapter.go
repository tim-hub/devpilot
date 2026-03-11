package taskrunner

import "fmt"

// AgentConfig holds the runtime configuration for a single agent.
type AgentConfig struct {
	Name  string // "claude", "gemini", "opencode", "cursor"
	Model string // optional model override
}

// AgentAdapter abstracts a headless CLI coding agent for use in the task runner.
// Each supported agent (claude, gemini, opencode, cursor) implements this interface.
type AgentAdapter interface {
	// Name returns the agent identifier, e.g. "claude", "gemini".
	Name() string

	// BuildCommand returns the binary and args to invoke the agent headlessly.
	// The prompt is embedded in the returned args by the adapter.
	BuildCommand(prompt string) (cmd string, args []string)

	// HandleLine is called for each line of stdout from the agent process.
	// The adapter parses its output format and emits the appropriate runner Events.
	HandleLine(line string, emit EventHandler)

	// FormatPrompt builds the agent-specific prompt for a task.
	// openspecDir is the project root; used by non-Claude adapters to read change content.
	FormatPrompt(task Task, useOpenSpec bool, openspecDir string) string
}

// NewAgentAdapter returns the AgentAdapter for the named agent, or an error if unknown.
func NewAgentAdapter(cfg AgentConfig) (AgentAdapter, error) {
	switch cfg.Name {
	case "claude", "":
		return newClaudeAdapter(cfg), nil
	case "gemini":
		return newGeminiAdapter(cfg), nil
	case "opencode":
		return newOpencodeAdapter(cfg), nil
	case "cursor":
		return newCursorAdapter(cfg), nil
	default:
		return nil, fmt.Errorf("unknown agent %q: supported agents are claude, gemini, opencode, cursor", cfg.Name)
	}
}

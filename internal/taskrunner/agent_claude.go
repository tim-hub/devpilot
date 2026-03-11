package taskrunner

import (
	"fmt"
	"os"
	"path/filepath"
)

// claudeAdapter implements AgentAdapter for Claude Code.
// Invokes `claude -p --output-format stream-json` for headless execution.
type claudeAdapter struct {
	cfg    AgentConfig
	bridge *eventBridge // stateful bridge reused across a runner's lifetime
}

func newClaudeAdapter(cfg AgentConfig) *claudeAdapter {
	return &claudeAdapter{cfg: cfg}
}

func (a *claudeAdapter) Name() string { return "claude" }

func (a *claudeAdapter) BuildCommand(prompt string) (string, []string) {
	args := []string{"-p", "--verbose", "--output-format", "stream-json", "--allowedTools=*"}
	if a.cfg.Model != "" {
		args = append(args, "--model", a.cfg.Model)
	}
	args = append(args, prompt)
	return "claude", args
}

// HandleLine parses Claude's stream-json output and emits runner Events.
// The eventBridge is created lazily on first call and reused for the runner's lifetime.
func (a *claudeAdapter) HandleLine(line string, emit EventHandler) {
	if a.bridge == nil {
		a.bridge = newEventBridge(emit)
	}
	event, err := ParseLine([]byte(line))
	if err == nil && event != nil {
		a.bridge.Handle(event)
	}
}

// FormatPrompt builds the Claude-specific prompt.
// Uses /opsx:apply skill when OpenSpec is detected; otherwise uses raw plan text.
func (a *claudeAdapter) FormatPrompt(task Task, useOpenSpec bool, openspecDir string) string {
	if useOpenSpec {
		return fmt.Sprintf(`Execute the following OpenSpec change autonomously from start to finish. This runs unattended — never stop to ask for feedback, confirmation, or approval.

Run: /opsx:apply %s

Rules:
- Execute ALL tasks without stopping
- Commit after each logical unit of work
- Never ask for user input or feedback
- If a task is blocked, skip it and continue with the next task
- When ALL tasks are complete, push to the current branch`, task.Name)
	}
	return fmt.Sprintf(`Execute the following task plan autonomously from start to finish. This runs unattended — never stop to ask for feedback, confirmation, or approval. Execute ALL steps/batches continuously without pausing.

Use /superpowers:test-driven-development and /superpowers:verification-before-completion skills during execution.

Task: %s

Plan:
%s

Rules:
- Execute ALL steps in the plan without stopping. Do NOT pause between batches or steps for review.
- Commit after each logical unit of work
- Never ask for user input or feedback
- If a step is blocked, skip it and continue with the next step
- When ALL steps are complete, push to the current branch`, task.Name, task.Description)
}

// loadOpenSpecContent reads proposal.md and tasks.md from the given OpenSpec change directory.
// Used by non-Claude adapters to inject raw change content into their prompts.
func loadOpenSpecContent(openspecDir string, changeName string) string {
	changeDir := filepath.Join(openspecDir, "openspec", "changes", changeName)

	readFile := func(name string) string {
		data, err := os.ReadFile(filepath.Join(changeDir, name))
		if err != nil {
			return ""
		}
		return string(data)
	}

	proposal := readFile("proposal.md")
	tasks := readFile("tasks.md")

	if proposal == "" && tasks == "" {
		return ""
	}

	var content string
	if proposal != "" {
		content += "## Proposal\n\n" + proposal + "\n\n"
	}
	if tasks != "" {
		content += "## Implementation Tasks\n\n" + tasks
	}
	return content
}

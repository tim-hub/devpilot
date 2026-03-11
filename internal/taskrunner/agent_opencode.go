package taskrunner

import "fmt"

// opencodeAdapter implements AgentAdapter for Opencode CLI.
// Invokes `opencode run --format json` for headless execution.
type opencodeAdapter struct {
	cfg    AgentConfig
	bridge *opencodeEventBridge
}

func newOpencodeAdapter(cfg AgentConfig) *opencodeAdapter {
	return &opencodeAdapter{cfg: cfg}
}

func (a *opencodeAdapter) Name() string { return "opencode" }

func (a *opencodeAdapter) BuildCommand(prompt string) (string, []string) {
	args := []string{"run", "--format", "json"}
	if a.cfg.Model != "" {
		args = append(args, "--model", a.cfg.Model)
	}
	args = append(args, prompt)
	return "opencode", args
}

func (a *opencodeAdapter) HandleLine(line string, emit EventHandler) {
	if a.bridge == nil {
		a.bridge = newOpencodeEventBridge(emit)
	}
	a.bridge.Handle(line)
}

func (a *opencodeAdapter) FormatPrompt(task Task, useOpenSpec bool, openspecDir string) string {
	if useOpenSpec {
		content := loadOpenSpecContent(openspecDir, task.Name)
		if content != "" {
			return fmt.Sprintf(`Execute the following OpenSpec change autonomously from start to finish. This runs unattended — never stop to ask for feedback, confirmation, or approval.

%s

Rules:
- Execute ALL tasks without stopping
- Commit after each logical unit of work
- Never ask for user input or feedback
- If a task is blocked, skip it and continue with the next task
- When ALL tasks are complete, push to the current branch`, content)
		}
	}
	return fmt.Sprintf(`Execute the following task plan autonomously from start to finish. This runs unattended — never stop to ask for feedback, confirmation, or approval.

Task: %s

Plan:
%s

Rules:
- Execute ALL steps in the plan without stopping
- Commit after each logical unit of work
- Never ask for user input or feedback
- If a step is blocked, skip it and continue with the next step
- When ALL steps are complete, push to the current branch`, task.Name, task.Description)
}

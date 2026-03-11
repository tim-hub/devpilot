package taskrunner

import "fmt"

// geminiAdapter implements AgentAdapter for Gemini CLI.
// Invokes `gemini -p --output-format stream-json --yolo` for headless execution.
type geminiAdapter struct {
	cfg    AgentConfig
	bridge *geminiEventBridge
}

func newGeminiAdapter(cfg AgentConfig) *geminiAdapter {
	return &geminiAdapter{cfg: cfg}
}

func (a *geminiAdapter) Name() string { return "gemini" }

func (a *geminiAdapter) BuildCommand(prompt string) (string, []string) {
	args := []string{"-p", "--output-format", "stream-json", "--yolo"}
	if a.cfg.Model != "" {
		args = append(args, "--model", a.cfg.Model)
	}
	args = append(args, prompt)
	return "gemini", args
}

func (a *geminiAdapter) HandleLine(line string, emit EventHandler) {
	if a.bridge == nil {
		a.bridge = newGeminiEventBridge(emit)
	}
	a.bridge.Handle(line)
}

func (a *geminiAdapter) FormatPrompt(task Task, useOpenSpec bool, openspecDir string) string {
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

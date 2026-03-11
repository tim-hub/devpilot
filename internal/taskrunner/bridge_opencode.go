package taskrunner

import (
	"encoding/json"
	"strings"
)

// opencodeEventBridge translates Opencode CLI's JSON output into runner Events.
// Opencode uses a step-based format: step_start, text, step_finish events.
type opencodeEventBridge struct {
	emit EventHandler
}

func newOpencodeEventBridge(emit EventHandler) *opencodeEventBridge {
	return &opencodeEventBridge{emit: emit}
}

// rawOpencodeEvent is the top-level Opencode JSON event.
type rawOpencodeEvent struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Tool    string `json:"tool"`
	StepID  string `json:"step_id"`
	Success bool   `json:"success"`
}

// Handle parses one line of Opencode output and emits runner Events.
// Opencode's step-based format provides less granularity than stream-json,
// so tool calls are surfaced as text events (graceful degradation).
func (b *opencodeEventBridge) Handle(line string) {
	data := []byte(line)

	var event rawOpencodeEvent
	if err := json.Unmarshal(data, &event); err != nil {
		// Non-JSON: emit as raw text.
		stripped := strings.TrimSpace(line)
		if stripped != "" {
			b.emit(TextOutputEvent{Text: stripped})
		}
		return
	}

	switch event.Type {
	case "text":
		if event.Text != "" {
			b.emit(TextOutputEvent{Text: event.Text})
		}
	case "step_start":
		// Opencode signals a tool step is starting.
		if event.Tool != "" {
			b.emit(ToolStartEvent{ToolName: event.Tool, Input: map[string]any{}})
		}
	case "step_finish":
		// Opencode signals a tool step has finished.
		if event.Tool != "" {
			b.emit(ToolResultEvent{ToolName: event.Tool})
		}
	}
}

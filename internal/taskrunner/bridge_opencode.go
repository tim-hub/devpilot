package taskrunner

import (
	"encoding/json"
	"strings"
)

// opencodeEventBridge translates opencode CLI's JSON output into runner Events.
//
// opencode run --format json emits one JSON object per line (NDJSON).
// Each line is a "part" with a top-level "type" field. Observed types:
//
//   - "text"        — assistant text output; field: text string
//   - "tool"        — tool call; fields: callID, tool, state.status, state.input/output
//   - "step-start"  — beginning of a reasoning step (no useful fields)
//   - "step-finish" — end of step; fields: reason, tokens.{input,output,total}
//
// Tool state machine: opencode emits the same callID multiple times as status
// progresses pending → running → completed/error. The bridge tracks in-flight
// tool IDs to emit ToolStartEvent once (on "running") and ToolResultEvent once
// (on "completed" or "error").
type opencodeEventBridge struct {
	emit         EventHandler
	inflightTools map[string]string // callID → tool name, for deduplication
}

func newOpencodeEventBridge(emit EventHandler) *opencodeEventBridge {
	return &opencodeEventBridge{
		emit:          emit,
		inflightTools: make(map[string]string),
	}
}

// rawOpencodeEvent is the top-level NDJSON line from opencode run --format json.
type rawOpencodeEvent struct {
	Type   string              `json:"type"`
	Text   string              `json:"text"`   // "text" events
	CallID string              `json:"callID"` // "tool" events
	Tool   string              `json:"tool"`   // "tool" events: tool name
	State  *opencodeToolState  `json:"state"`  // "tool" events
	Reason string              `json:"reason"` // "step-finish"
	Tokens *opencodeTokens     `json:"tokens"` // "step-finish"
}

type opencodeToolState struct {
	Status string `json:"status"` // "pending", "running", "completed", "error"
	Title  string `json:"title"`
	Output string `json:"output"`
}

type opencodeTokens struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

// Handle parses one line of opencode JSON output and emits runner Events.
func (b *opencodeEventBridge) Handle(line string) {
	data := []byte(line)

	var event rawOpencodeEvent
	if err := json.Unmarshal(data, &event); err != nil {
		// Non-JSON line: emit as raw text (startup messages, warnings, etc.)
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

	case "tool":
		if event.CallID == "" || event.Tool == "" || event.State == nil {
			return
		}
		switch event.State.Status {
		case "running":
			// Emit ToolStartEvent once when the tool begins executing.
			if _, seen := b.inflightTools[event.CallID]; !seen {
				b.inflightTools[event.CallID] = event.Tool
				b.emit(ToolStartEvent{ToolName: event.Tool, Input: map[string]any{}})
			}
		case "completed", "error":
			// Emit ToolResultEvent when the tool finishes.
			toolName := b.inflightTools[event.CallID]
			if toolName == "" {
				toolName = event.Tool
			}
			delete(b.inflightTools, event.CallID)
			b.emit(ToolResultEvent{ToolName: toolName})
		}

	case "step-finish":
		if event.Tokens != nil && (event.Tokens.Input > 0 || event.Tokens.Output > 0) {
			b.emit(StatsUpdateEvent{
				InputTokens:  event.Tokens.Input,
				OutputTokens: event.Tokens.Output,
			})
		}

	// "step-start" carries no useful info for the TUI — skip.
	}
}

package taskrunner

import (
	"encoding/json"
	"strings"
)

// cursorEventBridge translates Cursor Agent CLI's stream-json output into runner Events.
// Cursor's format is similar to Claude's stream-json.
type cursorEventBridge struct {
	emit          EventHandler
	inflightTools map[string]string // tool_use_id -> tool name
}

func newCursorEventBridge(emit EventHandler) *cursorEventBridge {
	return &cursorEventBridge{
		emit:          emit,
		inflightTools: make(map[string]string),
	}
}

// rawCursorEnvelope is the top-level Cursor stream-json message.
type rawCursorEnvelope struct {
	Type string `json:"type"`
}

// rawCursorAssistantMsg mirrors the assistant event structure.
type rawCursorAssistantMsg struct {
	Message struct {
		Content []json.RawMessage `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// rawCursorResultMsg mirrors the result event structure.
type rawCursorResultMsg struct {
	NumTurns int `json:"num_turns"`
	Usage    struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Handle parses one line of Cursor output and emits runner Events.
func (b *cursorEventBridge) Handle(line string) {
	data := []byte(line)

	var envelope rawCursorEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		stripped := strings.TrimSpace(line)
		if stripped != "" {
			b.emit(TextOutputEvent{Text: stripped})
		}
		return
	}

	switch envelope.Type {
	case "assistant":
		b.handleAssistant(data)
	case "result":
		b.handleResult(data)
	}
}

func (b *cursorEventBridge) handleAssistant(data []byte) {
	var raw rawCursorAssistantMsg
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}

	if raw.Message.Usage.InputTokens > 0 || raw.Message.Usage.OutputTokens > 0 {
		b.emit(StatsUpdateEvent{
			InputTokens:  raw.Message.Usage.InputTokens,
			OutputTokens: raw.Message.Usage.OutputTokens,
		})
	}

	type rawBlock struct {
		Type  string         `json:"type"`
		Text  string         `json:"text"`
		ID    string         `json:"id"`
		Name  string         `json:"name"`
		Input map[string]any `json:"input"`
	}

	for _, rawBlockData := range raw.Message.Content {
		var block rawBlock
		if err := json.Unmarshal(rawBlockData, &block); err != nil {
			continue
		}
		switch block.Type {
		case "text":
			if block.Text != "" {
				b.emit(TextOutputEvent{Text: block.Text})
			}
		case "tool_use":
			b.inflightTools[block.ID] = block.Name
			b.emit(ToolStartEvent{ToolName: block.Name, Input: block.Input})
		}
	}
}

func (b *cursorEventBridge) handleResult(data []byte) {
	var raw rawCursorResultMsg
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	b.emit(StatsUpdateEvent{
		InputTokens:  raw.Usage.InputTokens,
		OutputTokens: raw.Usage.OutputTokens,
		Turns:        raw.NumTurns,
	})
}

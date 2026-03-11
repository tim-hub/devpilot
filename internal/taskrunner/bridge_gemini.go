package taskrunner

import (
	"encoding/json"
	"strings"
)

// geminiEventBridge translates Gemini CLI's stream-json output into runner Events.
// Gemini's format closely mirrors Claude's but has subtle field differences.
type geminiEventBridge struct {
	emit          EventHandler
	inflightTools map[string]string // tool_use_id -> tool name
}

func newGeminiEventBridge(emit EventHandler) *geminiEventBridge {
	return &geminiEventBridge{
		emit:          emit,
		inflightTools: make(map[string]string),
	}
}

// rawGeminiEnvelope is the top-level Gemini stream-json message.
type rawGeminiEnvelope struct {
	Type string `json:"type"`
}

// rawGeminiAssistantMsg mirrors Claude's assistant format with Gemini's field names.
type rawGeminiAssistantMsg struct {
	Message struct {
		Content []json.RawMessage `json:"content"`
		Usage   struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// rawGeminiResultMsg mirrors Claude's result format.
type rawGeminiResultMsg struct {
	NumTurns int `json:"num_turns"`
	Usage    struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Handle parses one line of Gemini output and emits runner Events.
func (b *geminiEventBridge) Handle(line string) {
	data := []byte(line)

	var envelope rawGeminiEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		// Non-JSON: emit as raw text.
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

func (b *geminiEventBridge) handleAssistant(data []byte) {
	var raw rawGeminiAssistantMsg
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
		case "tool_result":
			toolName := b.inflightTools[block.ID]
			delete(b.inflightTools, block.ID)
			b.emit(ToolResultEvent{ToolName: toolName})
		}
	}
}

func (b *geminiEventBridge) handleResult(data []byte) {
	var raw rawGeminiResultMsg
	if err := json.Unmarshal(data, &raw); err != nil {
		return
	}
	b.emit(StatsUpdateEvent{
		InputTokens:  raw.Usage.InputTokens,
		OutputTokens: raw.Usage.OutputTokens,
		Turns:        raw.NumTurns,
	})
}

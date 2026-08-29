package relayconvert

import "strings"

// ClaudeToOpenAIRequest converts an Anthropic Messages request to an OpenAI chat
// request: system → system message, content blocks → text/parts, tool_use →
// assistant tool_calls, tool_result → tool role.
func ClaudeToOpenAIRequest(req *ClaudeRequest) *OpenAIChatRequest {
	out := &OpenAIChatRequest{Model: req.Model, Stream: req.Stream, MaxTokens: intPtr(req.MaxTokens), Temperature: req.Temperature, TopP: req.TopP}
	if req.System != "" {
		out.Messages = append(out.Messages, OpenAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "assistant":
			var text strings.Builder
			msg := OpenAIMessage{Role: "assistant"}
			for _, b := range claudeContentBlocks(m.Content) {
				switch b.Type {
				case "text":
					text.WriteString(b.Text)
				case "tool_use":
					fn := OpenAIFunction{Name: b.Name, Arguments: stringifyJSON(b.Input)}
					msg.ToolCalls = append(msg.ToolCalls, OpenAIToolCall{ID: b.ID, Type: "function", Function: fn})
				}
			}
			msg.Content = text.String()
			if msg.Content != "" || len(msg.ToolCalls) > 0 {
				out.Messages = append(out.Messages, msg)
			}
		default:
			out.Messages = append(out.Messages, OpenAIMessage{Role: "user", Content: claudeContentToText(m.Content)})
		}
	}
	return out
}

func claudeContentBlocks(content any) []ClaudeContentBlock {
	switch v := content.(type) {
	case []ClaudeContentBlock:
		return v
	case []any:
		out := make([]ClaudeContentBlock, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				b := ClaudeContentBlock{}
				if typ, _ := m["type"].(string); typ != "" {
					b.Type = typ
				}
				if t, _ := m["text"].(string); t != "" {
					b.Text = t
				}
				if id, _ := m["id"].(string); id != "" {
					b.ID = id
				}
				if name, _ := m["name"].(string); name != "" {
					b.Name = name
				}
				b.Input = m["input"]
				out = append(out, b)
			}
		}
		return out
	default:
		return nil
	}
}

func claudeContentToText(content any) string {
	var b strings.Builder
	switch v := content.(type) {
	case string:
		return v
	case []ClaudeContentBlock:
		for _, c := range v {
			if c.Type == "text" {
				b.WriteString(c.Text)
			}
		}
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, _ := m["text"].(string); t != "" {
						b.WriteString(t)
					}
				}
			}
		}
	}
	return b.String()
}

// OpenAIToClaudeResponse converts an OpenAI chat completion response to an
// Anthropic Messages response (non-streaming).
func OpenAIToClaudeResponse(oai *OpenAIChatResponse) *ClaudeResponse {
	choice := oai.Choices[0]
	content := []ClaudeContentBlock{}
	if choice.Message.Content != "" {
		content = append(content, ClaudeContentBlock{Type: "text", Text: choice.Message.Content})
	}
	for _, tc := range choice.Message.ToolCalls {
		content = append(content, ClaudeContentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: parseJSON(tc.Function.Arguments)})
	}
	stop := "end_turn"
	switch choice.FinishReason {
	case "length":
		stop = "max_tokens"
	case "tool_calls":
		stop = "tool_use"
	case "content_filter":
		stop = "refusal"
	}
	return &ClaudeResponse{
		ID: oai.ID, Type: "message", Role: "assistant", Model: oai.Model,
		Content: content, StopReason: stop,
		Usage: ClaudeUsage{InputTokens: oai.Usage.PromptTokens, OutputTokens: oai.Usage.CompletionTokens},
	}
}

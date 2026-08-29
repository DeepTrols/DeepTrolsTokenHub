package relayconvert

import (
	"strings"
	"time"
)

func intPtr(i int) *int { return &i }

// OpenAIToClaudeRequest converts an OpenAI chat request to an Anthropic Messages
// request: system messages are hoisted, tool_calls map to tool_use, and tool
// results map to tool_result user turns.
func OpenAIToClaudeRequest(req *OpenAIChatRequest) *ClaudeRequest {
	out := &ClaudeRequest{
		Model:       req.Model,
		MaxTokens:   1024,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		out.MaxTokens = *req.MaxTokens
	}
	if sl, ok := req.Stop.([]string); ok {
		out.StopSequences = sl
	} else if s, ok := req.Stop.(string); ok && s != "" {
		out.StopSequences = []string{s}
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, ClaudeTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	for _, m := range req.Messages {
		switch strings.ToLower(m.Role) {
		case "system":
			if s, ok := m.Content.(string); ok {
				if out.System != "" {
					out.System += "\n\n" + s
				} else {
					out.System = s
				}
			}
			continue
		case "tool":
			// tool role → user turn carrying a tool_result content block.
			out.Messages = append(out.Messages, ClaudeMessage{
				Role:    "user",
				Content: []ClaudeContentBlock{{Type: "tool_result", ID: m.ToolCallID, Content: toolText(m.Content)}},
			})
			continue
		}
		blocks := []ClaudeContentBlock{}
		switch c := m.Content.(type) {
		case string:
			if c != "" {
				blocks = append(blocks, ClaudeContentBlock{Type: "text", Text: c})
			}
		case []any:
			for _, item := range c {
				blocks = append(blocks, openAIItemToClaudeBlock(item))
			}
		}
		for _, tc := range m.ToolCalls {
			blocks = append(blocks, ClaudeContentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: parseJSON(tc.Function.Arguments)})
		}
		if len(blocks) == 0 {
			continue
		}
		out.Messages = append(out.Messages, ClaudeMessage{Role: roleAlias(m.Role), Content: blocks})
	}
	return out
}

func toolText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func roleAlias(role string) string {
	switch strings.ToLower(role) {
	case "assistant":
		return "assistant"
	default:
		return "user"
	}
}

// ClaudeToOpenAIChatResponse converts an Anthropic Messages response to an
// OpenAI chat completion (non-streaming), normalizing usage and stop_reason.
func ClaudeToOpenAIChatResponse(claude *ClaudeResponse) *OpenAIChatResponse {
	msg := OAIResponseMessage{Role: "assistant", Content: ""}
	var text strings.Builder
	for _, b := range claude.Content {
		switch b.Type {
		case "text":
			text.WriteString(b.Text)
		case "tool_use":
			msg.ToolCalls = append(msg.ToolCalls, OpenAIToolCall{
				ID: b.ID, Type: "function",
				Function: OpenAIFunction{Name: b.Name, Arguments: stringifyJSON(b.Input)},
			})
		}
	}
	msg.Content = text.String()
	resp := &OpenAIChatResponse{
		ID:      claude.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   claude.Model,
		Usage: OAIUsage{
			PromptTokens:     claude.Usage.InputTokens,
			CompletionTokens: claude.Usage.OutputTokens,
			TotalTokens:      claude.Usage.InputTokens + claude.Usage.OutputTokens,
		},
	}
	resp.Choices = []OAIChoice{{Index: 0, Message: msg, FinishReason: finishReason(claude.StopReason)}}
	return resp
}

func finishReason(stop string) string {
	switch stop {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

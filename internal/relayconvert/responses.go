package relayconvert

import "strings"

// Responses types (OpenAI Responses API, subset).
type ResponsesItem struct {
	Type    string             `json:"type,omitempty"`
	Role    string             `json:"role,omitempty"`
	Status  string             `json:"status,omitempty"`
	Content []ResponsesContent `json:"content,omitempty"`
	Name    string             `json:"name,omitempty"`
	CallID  string             `json:"call_id,omitempty"`
	Call    *ResponsesToolCall `json:"call,omitempty"`
}

type ResponsesContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type ResponsesToolCall struct {
	ID        string `json:"call_id,omitempty"`
	Name      string `json:"name"`
	Arguments any    `json:"arguments,omitempty"`
}

type ResponsesRequest struct {
	Model           string          `json:"model"`
	Input           []ResponsesItem `json:"input"`
	Instructions    string          `json:"instructions,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	MaxOutputTokens *int            `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
}

type ResponsesResponse struct {
	ID     string          `json:"id"`
	Object string          `json:"object"`
	Model  string          `json:"model"`
	Status string          `json:"status"`
	Output []ResponsesItem `json:"output"`
	Usage  ResponsesUsage  `json:"usage"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// OpenAIToResponsesRequest converts an OpenAI Chat Completions request to a
// Responses request: system messages become instructions, user/assistant text
// become input items, and tool_calls/tool results map to function call items.
func OpenAIToResponsesRequest(req *OpenAIChatRequest) *ResponsesRequest {
	out := &ResponsesRequest{
		Model:           req.Model,
		Stream:          req.Stream,
		MaxOutputTokens: req.MaxTokens,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
	}
	for _, m := range req.Messages {
		switch strings.ToLower(m.Role) {
		case "system":
			if s, ok := m.Content.(string); ok {
				if out.Instructions != "" {
					out.Instructions += "\n\n" + s
				} else {
					out.Instructions = s
				}
			}
		case "assistant":
			item := ResponsesItem{Type: "message", Role: "assistant"}
			if tc := toolCallsToResponses(m.ToolCalls); len(tc) > 0 {
				item.Type = "function_call"
				item.Name = tc[0].Name
				item.CallID = tc[0].ID
				item.Call = tc[0]
			} else if s, ok := m.Content.(string); ok && s != "" {
				item.Content = []ResponsesContent{{Type: "output_text", Text: s}}
			} else {
				continue
			}
			out.Input = append(out.Input, item)
		case "tool":
			out.Input = append(out.Input, ResponsesItem{Type: "function_call_output", CallID: m.ToolCallID, Content: []ResponsesContent{{Type: "output_text", Text: toolText(m.Content)}}})
		default:
			s := ""
			if v, ok := m.Content.(string); ok {
				s = v
			}
			out.Input = append(out.Input, ResponsesItem{Type: "message", Role: "user", Content: []ResponsesContent{{Type: "input_text", Text: s}}})
		}
	}
	return out
}

func toolCallsToResponses(toolCalls []OpenAIToolCall) []*ResponsesToolCall {
	var out []*ResponsesToolCall
	for _, tc := range toolCalls {
		out = append(out, &ResponsesToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: parseJSON(tc.Function.Arguments)})
	}
	return out
}

// ResponsesToOpenAIChatResponse converts a Responses response to an OpenAI chat
// completion (non-streaming), mapping output message content and usage.
func ResponsesToOpenAIChatResponse(resp *ResponsesResponse) *OpenAIChatResponse {
	msg := OAIResponseMessage{Role: "assistant"}
	var text strings.Builder
	for _, item := range resp.Output {
		if item.Type != "message" {
			continue
		}
		for _, c := range item.Content {
			if c.Type == "output_text" {
				text.WriteString(c.Text)
			}
		}
	}
	msg.Content = text.String()
	return &OpenAIChatResponse{
		ID: resp.ID, Object: "chat.completion", Model: resp.Model,
		Usage:   OAIUsage{PromptTokens: resp.Usage.InputTokens, CompletionTokens: resp.Usage.OutputTokens, TotalTokens: resp.Usage.InputTokens + resp.Usage.OutputTokens},
		Choices: []OAIChoice{{Index: 0, Message: msg, FinishReason: "stop"}},
	}
}

// ResponsesRequestToOpenAIChatRequest converts an OpenAI Responses request to a
// Chat Completions request: instructions → system message, input messages →
// chat messages, function_call → assistant tool_calls, function_call_output →
// tool results. Used when the routed upstream only exposes chat/completions
// (new-api responses_via_chat parity).
func ResponsesRequestToOpenAIChatRequest(req *ResponsesRequest) *OpenAIChatRequest {
	out := &OpenAIChatRequest{
		Model:       req.Model,
		Stream:      req.Stream,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
	}
	if req.Instructions != "" {
		out.Messages = append(out.Messages, OpenAIMessage{Role: "system", Content: req.Instructions})
	}
	for _, item := range req.Input {
		switch item.Type {
		case "function_call":
			msg := OpenAIMessage{Role: "assistant"}
			if item.Call != nil {
				args := ""
				if s, ok := item.Call.Arguments.(string); ok {
					args = s
				} else if item.Call.Arguments != nil {
					args = stringifyJSON(item.Call.Arguments)
				}
				msg.ToolCalls = append(msg.ToolCalls, OpenAIToolCall{
					ID: item.Call.ID, Type: "function",
					Function: OpenAIFunction{Name: item.Call.Name, Arguments: args},
				})
			}
			out.Messages = append(out.Messages, msg)
		case "function_call_output":
			out.Messages = append(out.Messages, OpenAIMessage{
				Role:       "tool",
				ToolCallID: item.CallID,
				Content:    responsesContentText(item.Content),
			})
		case "message":
			role := item.Role
			if role == "" {
				role = "user"
			}
			out.Messages = append(out.Messages, OpenAIMessage{
				Role:    role,
				Content: responsesContentText(item.Content),
			})
		}
	}
	return out
}

// responsesContentText flattens Responses content blocks to plain text.
func responsesContentText(content []ResponsesContent) string {
	var b strings.Builder
	for _, c := range content {
		b.WriteString(c.Text)
	}
	return b.String()
}

// OpenAIChatResponseToResponses converts an OpenAI Chat Completions response
// to an OpenAI Responses response (non-streaming), mapping the assistant
// message/tool calls and usage. new-api relaykit chat → responses parity.
func OpenAIChatResponseToResponses(oai *OpenAIChatResponse) *ResponsesResponse {
	out := &ResponsesResponse{
		ID:     oai.ID,
		Object: "response",
		Model:  oai.Model,
		Status: "completed",
		Usage: ResponsesUsage{
			InputTokens:  oai.Usage.PromptTokens,
			OutputTokens: oai.Usage.CompletionTokens,
			TotalTokens:  oai.Usage.PromptTokens + oai.Usage.CompletionTokens,
		},
	}
	if len(oai.Choices) == 0 {
		return out
	}
	choice := oai.Choices[0]
	switch choice.FinishReason {
	case "length", "content_filter":
		out.Status = "incomplete"
	}
	if len(choice.Message.ToolCalls) > 0 {
		for _, tc := range choice.Message.ToolCalls {
			call := &ResponsesToolCall{ID: tc.ID, Name: tc.Function.Name, Arguments: parseJSON(tc.Function.Arguments)}
			out.Output = append(out.Output, ResponsesItem{
				Type:   "function_call",
				Name:   tc.Function.Name,
				CallID: tc.ID,
				Call:   call,
			})
		}
		return out
	}
	out.Output = append(out.Output, ResponsesItem{
		Type: "message", Role: "assistant", Status: "completed",
		Content: []ResponsesContent{{Type: "output_text", Text: choice.Message.Content}},
	})
	return out
}

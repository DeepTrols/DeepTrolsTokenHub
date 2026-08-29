package relayconvert

import "strings"

// Gemini generateContent types (subset).
type GeminiPart struct {
	Text             string      `json:"text,omitempty"`
	FunctionCall     *GeminiFunc `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunc `json:"functionResponse,omitempty"`
}

type GeminiFunc struct {
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

type GeminiRequest struct {
	Contents          []GeminiContent        `json:"contents"`
	SystemInstruction *GeminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  GeminiGenerationConfig `json:"generationConfig"`
}

type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type GeminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type GeminiResponse struct {
	Candidates    []GeminiCandidate `json:"candidates"`
	UsageMetadata GeminiUsage       `json:"usageMetadata"`
}

// OpenAIToGeminiRequest converts an OpenAI chat request to a Gemini generateContent
// request: system → systemInstruction, user/assistant → role user/model, and
// tool calls/results → functionCall/functionResponse parts.
func OpenAIToGeminiRequest(req *OpenAIChatRequest) *GeminiRequest {
	out := &GeminiRequest{GenerationConfig: GeminiGenerationConfig{Temperature: req.Temperature, TopP: req.TopP, MaxOutputTokens: req.MaxTokens}}
	for _, m := range req.Messages {
		switch strings.ToLower(m.Role) {
		case "system":
			if s, ok := m.Content.(string); ok {
				out.SystemInstruction = &GeminiContent{Role: "system", Parts: []GeminiPart{{Text: s}}}
			}
		case "assistant":
			parts := []GeminiPart{}
			if tc := m.ToolCalls; len(tc) > 0 {
				parts = append(parts, GeminiPart{FunctionCall: &GeminiFunc{Name: tc[0].Function.Name, Args: parseJSON(tc[0].Function.Arguments)}})
			} else if s, ok := m.Content.(string); ok && s != "" {
				parts = append(parts, GeminiPart{Text: s})
			}
			if len(parts) > 0 {
				out.Contents = append(out.Contents, GeminiContent{Role: "model", Parts: parts})
			}
		case "tool":
			out.Contents = append(out.Contents, GeminiContent{Role: "user", Parts: []GeminiPart{{FunctionResponse: &GeminiFunc{Name: m.Name, Args: map[string]any{"output": toolText(m.Content)}}}}})
		default:
			out.Contents = append(out.Contents, GeminiContent{Role: "user", Parts: []GeminiPart{{Text: toolText(m.Content)}}})
		}
	}
	return out
}

// GeminiToOpenAIChatResponse converts a Gemini generateContent response to an
// OpenAI chat completion (non-streaming), normalizing finishReason and usage.
func GeminiToOpenAIChatResponse(resp *GeminiResponse, model string) *OpenAIChatResponse {
	msg := OAIResponseMessage{Role: "assistant"}
	var text strings.Builder
	for _, p := range resp.Candidates[0].Content.Parts {
		if p.Text != "" {
			text.WriteString(p.Text)
		}
		if p.FunctionCall != nil {
			msg.ToolCalls = append(msg.ToolCalls, OpenAIToolCall{ID: "call_" + p.FunctionCall.Name, Type: "function", Function: OpenAIFunction{Name: p.FunctionCall.Name, Arguments: stringifyJSON(p.FunctionCall.Args)}})
		}
	}
	msg.Content = text.String()
	fr := "stop"
	switch resp.Candidates[0].FinishReason {
	case "MAX_TOKENS":
		fr = "length"
	case "SAFETY", "RECITATION":
		fr = "content_filter"
	}
	return &OpenAIChatResponse{
		ID: "chatcmpl-gemini", Object: "chat.completion", Model: model,
		Usage:   OAIUsage{PromptTokens: resp.UsageMetadata.PromptTokenCount, CompletionTokens: resp.UsageMetadata.CandidatesTokenCount, TotalTokens: resp.UsageMetadata.TotalTokenCount},
		Choices: []OAIChoice{{Index: 0, Message: msg, FinishReason: fr}},
	}
}

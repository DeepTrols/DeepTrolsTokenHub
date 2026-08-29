package relayconvert

import "testing"

func TestClaudeToOpenAIRequest(t *testing.T) {
	req := &ClaudeRequest{
		Model: "claude-sonnet-4-5", MaxTokens: 128, System: "sys",
		Messages: []ClaudeMessage{
			{Role: "user", Content: []ClaudeContentBlock{{Type: "text", Text: "hi"}}},
			{Role: "assistant", Content: []ClaudeContentBlock{{Type: "tool_use", ID: "t1", Name: "f", Input: map[string]any{"x": 1}}}},
		},
	}
	oai := ClaudeToOpenAIRequest(req)
	if oai.Messages[0].Role != "system" || oai.Messages[0].Content != "sys" {
		t.Fatalf("system: %+v", oai.Messages[0])
	}
	if oai.Messages[2].Role != "assistant" || len(oai.Messages[2].ToolCalls) != 1 {
		t.Fatalf("assistant tool_calls: %+v", oai.Messages[2])
	}
}

func TestOpenAIToClaudeResponse(t *testing.T) {
	oai := &OpenAIChatResponse{
		ID: "chatcmpl_1", Model: "m",
		Choices: []OAIChoice{{Index: 0, Message: OAIResponseMessage{Role: "assistant", Content: "hi", ToolCalls: []OpenAIToolCall{{ID: "t1", Type: "function", Function: OpenAIFunction{Name: "f", Arguments: `{"x":1}`}}}}, FinishReason: "tool_calls"}},
		Usage:   OAIUsage{PromptTokens: 5, CompletionTokens: 2},
	}
	c := OpenAIToClaudeResponse(oai)
	if c.StopReason != "tool_use" {
		t.Fatalf("stop_reason: %s", c.StopReason)
	}
	if len(c.Content) != 2 || c.Content[1].Type != "tool_use" {
		t.Fatalf("content: %+v", c.Content)
	}
	if c.Usage.InputTokens != 5 || c.Usage.OutputTokens != 2 {
		t.Fatalf("usage: %+v", c.Usage)
	}
}

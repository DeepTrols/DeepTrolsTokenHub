package relayconvert

import (
	"strings"
	"testing"
)

func TestOpenAIToClaudeRequest(t *testing.T) {
	req := &OpenAIChatRequest{
		Model: "claude-sonnet-4-5",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "", ToolCalls: []OpenAIToolCall{{ID: "call_1", Type: "function", Function: OpenAIFunction{Name: "get_weather", Arguments: `{"city":"SH"}`}}}},
			{Role: "tool", Content: "sunny", ToolCallID: "call_1"},
		},
	}
	c := OpenAIToClaudeRequest(req)
	if c.Model != "claude-sonnet-4-5" {
		t.Fatalf("model: %s", c.Model)
	}
	if c.System != "You are helpful" {
		t.Fatalf("system not hoisted: %q", c.System)
	}
	if len(c.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(c.Messages))
	}
	if claudeContentBlocks(c.Messages[1].Content)[0].Type != "tool_use" {
		t.Fatalf("expected tool_use block, got %s", claudeContentBlocks(c.Messages[1].Content)[0].Type)
	}
	if c.Messages[2].Role != "user" || claudeContentBlocks(c.Messages[2].Content)[0].Type != "tool_result" {
		t.Fatalf("expected tool_result user msg, got %+v", c.Messages[2])
	}
}

func TestClaudeToOpenAIChatResponse(t *testing.T) {
	claude := &ClaudeResponse{
		ID: "msg_1", Type: "message", Role: "assistant", Model: "claude-sonnet-4-5",
		StopReason: "tool_use",
		Content: []ClaudeContentBlock{
			{Type: "text", Text: "checking"},
			{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: map[string]any{"city": "SH"}},
		},
		Usage: ClaudeUsage{InputTokens: 10, OutputTokens: 5},
	}
	oai := ClaudeToOpenAIChatResponse(claude)
	if oai.Choices[0].Message.Content != "checking" {
		t.Fatalf("content: %q", oai.Choices[0].Message.Content)
	}
	if oai.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish_reason: %s", oai.Choices[0].FinishReason)
	}
	if len(oai.Choices[0].Message.ToolCalls) != 1 || oai.Choices[0].Message.ToolCalls[0].ID != "toolu_1" {
		t.Fatalf("tool_calls: %+v", oai.Choices[0].Message.ToolCalls)
	}
	if oai.Usage.TotalTokens != 15 || oai.Usage.PromptTokens != 10 || oai.Usage.CompletionTokens != 5 {
		t.Fatalf("usage: %+v", oai.Usage)
	}
	if !strings.Contains(oai.Choices[0].Message.ToolCalls[0].Function.Arguments, "SH") {
		t.Fatalf("arguments not preserved: %s", oai.Choices[0].Message.ToolCalls[0].Function.Arguments)
	}
}

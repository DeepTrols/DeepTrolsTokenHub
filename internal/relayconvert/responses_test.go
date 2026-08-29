package relayconvert

import (
	"strings"
	"testing"
)

func TestOpenAIToResponsesRequest(t *testing.T) {
	req := &OpenAIChatRequest{
		Model: "gpt-5",
		Messages: []OpenAIMessage{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hi"},
		},
		MaxTokens: intPtr(64),
	}
	r := OpenAIToResponsesRequest(req)
	if r.Instructions != "be concise" {
		t.Fatalf("instructions: %q", r.Instructions)
	}
	if len(r.Input) != 1 || r.Input[0].Content[0].Text != "hi" {
		t.Fatalf("input: %+v", r.Input)
	}
	if r.MaxOutputTokens == nil || *r.MaxOutputTokens != 64 {
		t.Fatalf("max_output_tokens: %+v", r.MaxOutputTokens)
	}
}

func TestResponsesToOpenAIChatResponse(t *testing.T) {
	resp := &ResponsesResponse{
		ID: "resp_1", Model: "gpt-5",
		Output: []ResponsesItem{{Type: "message", Role: "assistant", Content: []ResponsesContent{{Type: "output_text", Text: "hello"}}}},
		Usage:  ResponsesUsage{InputTokens: 10, OutputTokens: 4},
	}
	oai := ResponsesToOpenAIChatResponse(resp)
	if oai.Choices[0].Message.Content != "hello" {
		t.Fatalf("content: %q", oai.Choices[0].Message.Content)
	}
	if oai.Usage.TotalTokens != 14 {
		t.Fatalf("usage: %+v", oai.Usage)
	}
}

func TestResponsesRequestToOpenAIChatRequest(t *testing.T) {
	req := &ResponsesRequest{
		Model:        "deepseek-chat",
		Instructions: "be concise",
		Stream:       true,
		Input: []ResponsesItem{
			{Type: "message", Role: "user", Content: []ResponsesContent{{Type: "input_text", Text: "hi"}}},
			{Type: "function_call", Name: "get_weather", CallID: "call_1", Call: &ResponsesToolCall{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Shanghai"}}},
			{Type: "function_call_output", CallID: "call_1", Content: []ResponsesContent{{Type: "output_text", Text: `{"temp":28}`}}},
		},
		MaxOutputTokens: intPtr(64),
	}
	oai := ResponsesRequestToOpenAIChatRequest(req)
	if oai.Model != "deepseek-chat" || !oai.Stream {
		t.Fatalf("model/stream not preserved: %+v", oai)
	}
	if len(oai.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d: %+v", len(oai.Messages), oai.Messages)
	}
	if oai.Messages[0].Role != "system" || oai.Messages[0].Content != "be concise" {
		t.Fatalf("instructions should become a system message: %+v", oai.Messages[0])
	}
	if oai.Messages[1].Role != "user" || oai.Messages[1].Content != "hi" {
		t.Fatalf("user message: %+v", oai.Messages[1])
	}
	assistant := oai.Messages[2]
	if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 {
		t.Fatalf("function_call should map to assistant tool_calls: %+v", assistant)
	}
	if assistant.ToolCalls[0].ID != "call_1" || assistant.ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("tool call mapping: %+v", assistant.ToolCalls[0])
	}
	if !strings.Contains(assistant.ToolCalls[0].Function.Arguments, "Shanghai") {
		t.Fatalf("arguments should be JSON: %q", assistant.ToolCalls[0].Function.Arguments)
	}
	if oai.Messages[3].Role != "tool" || oai.Messages[3].ToolCallID != "call_1" {
		t.Fatalf("function_call_output should map to tool message: %+v", oai.Messages[3])
	}
	if oai.MaxTokens == nil || *oai.MaxTokens != 64 {
		t.Fatalf("max_output_tokens → max_tokens: %+v", oai.MaxTokens)
	}
}

func TestOpenAIChatResponseToResponses(t *testing.T) {
	oai := &OpenAIChatResponse{
		ID: "chatcmpl-1", Object: "chat.completion", Model: "deepseek-chat",
		Choices: []OAIChoice{{Index: 0, Message: OAIResponseMessage{Role: "assistant", Content: "hello"}, FinishReason: "stop"}},
		Usage:   OAIUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}
	resp := OpenAIChatResponseToResponses(oai)
	if resp.ID != "chatcmpl-1" || resp.Object != "response" || resp.Status != "completed" {
		t.Fatalf("envelope: %+v", resp)
	}
	if len(resp.Output) != 1 || resp.Output[0].Type != "message" {
		t.Fatalf("output: %+v", resp.Output)
	}
	item := resp.Output[0]
	if len(item.Content) != 1 || item.Content[0].Type != "output_text" || item.Content[0].Text != "hello" {
		t.Fatalf("content: %+v", item.Content)
	}
	if resp.Usage.InputTokens != 10 || resp.Usage.OutputTokens != 4 || resp.Usage.TotalTokens != 14 {
		t.Fatalf("usage: %+v", resp.Usage)
	}
}

func TestOpenAIChatResponseToResponses_ToolCalls(t *testing.T) {
	oai := &OpenAIChatResponse{
		ID: "chatcmpl-2", Object: "chat.completion", Model: "deepseek-chat",
		Choices: []OAIChoice{{
			Index: 0,
			Message: OAIResponseMessage{
				Role:      "assistant",
				ToolCalls: []OpenAIToolCall{{ID: "call_1", Type: "function", Function: OpenAIFunction{Name: "get_weather", Arguments: `{"city":"Shanghai"}`}}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: OAIUsage{PromptTokens: 5, CompletionTokens: 2},
	}
	resp := OpenAIChatResponseToResponses(oai)
	if len(resp.Output) != 1 || resp.Output[0].Type != "function_call" {
		t.Fatalf("expected function_call output: %+v", resp.Output)
	}
	call := resp.Output[0]
	if call.Name != "get_weather" || call.CallID != "call_1" {
		t.Fatalf("call mapping: %+v", call)
	}
	if call.Call == nil {
		t.Fatal("call details missing")
	}
	if args, ok := call.Call.Arguments.(map[string]any); !ok || args["city"] != "Shanghai" {
		t.Fatalf("arguments should be parsed JSON: %+v", call.Call.Arguments)
	}
}

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/pkg/usageparser"
)

func TestAnthropicAdapter_ExecuteEndpoint(t *testing.T) {
	var gotPath string
	var gotHeaders http.Header
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4",
			"content":[{"type":"text","text":"hey"}],"stop_reason":"end_turn",
			"usage":{"input_tokens":8,"output_tokens":3}
		}`))
	}))
	defer upstream.Close()

	adapter := NewAnthropicAdapter()
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "anthropic-key",
		"claude-sonnet-4", "chat/completions", map[string]any{
			"model": "claude-sonnet-4",
			"messages": []any{
				map[string]any{"role": "system", "content": "sys"},
				map[string]any{"role": "user", "content": "hi"},
			},
		})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("path = %q, want /v1/messages", gotPath)
	}
	if gotHeaders.Get("x-api-key") != "anthropic-key" {
		t.Fatalf("x-api-key = %q", gotHeaders.Get("x-api-key"))
	}
	if gotHeaders.Get("anthropic-version") != anthropicAPIVersion {
		t.Fatalf("anthropic-version = %q", gotHeaders.Get("anthropic-version"))
	}
	// Request must be Claude-shaped.
	if gotBody["system"] != "sys" {
		t.Fatalf("expected system hoisting, got %v", gotBody)
	}
	messages, _ := gotBody["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected 1 claude message, got %v", gotBody)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	choices := resp.Body["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "hey" {
		t.Fatalf("content = %v, want hey", msg["content"])
	}
	if choices[0].(map[string]any)["finish_reason"] != "stop" {
		t.Fatalf("finish_reason = %v, want stop", choices[0].(map[string]any)["finish_reason"])
	}
	if resp.Usage.TotalTokens != 11 || resp.UsageSource != usageparser.SourceUpstream {
		t.Fatalf("usage = %+v source = %s", resp.Usage, resp.UsageSource)
	}
}

func TestAnthropicAdapter_MaxTokensFinishReason(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_2","type":"message","role":"assistant","model":"claude-sonnet-4",
			"content":[{"type":"text","text":"partial"}],"stop_reason":"max_tokens",
			"usage":{"input_tokens":5,"output_tokens":2}
		}`))
	}))
	defer upstream.Close()

	adapter := NewAnthropicAdapter()
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k", "claude-sonnet-4", "chat/completions", map[string]any{
		"model":    "claude-sonnet-4",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	choice := resp.Body["choices"].([]any)[0].(map[string]any)
	if choice["finish_reason"] != "length" {
		t.Fatalf("finish_reason = %v, want length for max_tokens", choice["finish_reason"])
	}
}

func TestAnthropicAdapter_ToolCalls(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_3","type":"message","role":"assistant","model":"claude-sonnet-4",
			"content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Shanghai"}}],
			"stop_reason":"tool_use","usage":{"input_tokens":4,"output_tokens":1}
		}`))
	}))
	defer upstream.Close()

	adapter := NewAnthropicAdapter()
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k", "claude-sonnet-4", "chat/completions", map[string]any{
		"model":    "claude-sonnet-4",
		"messages": []any{map[string]any{"role": "user", "content": "weather?"}},
	})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	msg := resp.Body["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	toolCalls := msg["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected tool_calls, got %v", msg)
	}
	fn := toolCalls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "get_weather" {
		t.Fatalf("tool name = %v", fn["name"])
	}
}

func TestAnthropicMessagesURL(t *testing.T) {
	cases := map[string]string{
		"https://api.anthropic.com":     "https://api.anthropic.com/v1/messages",
		"https://api.anthropic.com/v1":  "https://api.anthropic.com/v1/messages",
		"https://api.anthropic.com/v1/": "https://api.anthropic.com/v1/messages",
	}
	for base, want := range cases {
		if got := anthropicMessagesURL(base); got != want {
			t.Errorf("anthropicMessagesURL(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestAnthropicAdapter_RejectsUnsupportedEndpoints(t *testing.T) {
	adapter := NewAnthropicAdapter()
	if _, err := adapter.ExecuteEndpointRaw(context.Background(), "u", "k", "m", "audio/speech", nil); err == nil {
		t.Fatal("expected raw endpoint error")
	}
}

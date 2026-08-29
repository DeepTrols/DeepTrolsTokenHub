package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/pkg/usageparser"
)

func TestOllamaAdapter_ExecuteEndpoint(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"llama3","created_at":"2026-08-29T00:00:00Z",
			"message":{"role":"assistant","content":"hey from ollama"},"done":true,
			"prompt_eval_count":12,"eval_count":8
		}`))
	}))
	defer upstream.Close()

	adapter := NewOllamaAdapter()
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "",
		"llama3", "chat/completions", map[string]any{
			"model": "llama3",
			"messages": []any{
				map[string]any{"role": "system", "content": "sys"},
				map[string]any{"role": "user", "content": "hi"},
			},
			"temperature": 0.5,
		})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if gotPath != "/api/chat" {
		t.Fatalf("path = %q, want /api/chat", gotPath)
	}
	if gotBody["model"] != "llama3" || gotBody["stream"] != false {
		t.Fatalf("request envelope: %v", gotBody)
	}
	messages, _ := gotBody["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %v", gotBody)
	}
	sys := messages[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "sys" {
		t.Fatalf("system message: %v", sys)
	}
	options, _ := gotBody["options"].(map[string]any)
	if options["temperature"] != 0.5 {
		t.Fatalf("options: %v", options)
	}
	choices := resp.Body["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "hey from ollama" {
		t.Fatalf("content = %v", msg["content"])
	}
	if resp.Usage.TotalTokens != 20 || resp.UsageSource != usageparser.SourceUpstream {
		t.Fatalf("usage = %+v source = %s", resp.Usage, resp.UsageSource)
	}
	usageMap := resp.Body["usage"].(map[string]any)
	if usageMap["total_tokens"] != 20 {
		t.Fatalf("usage map: %v", usageMap)
	}
}

func TestOllamaAdapter_FlattensContentBlocks(t *testing.T) {
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ok"},"done":true}`))
	}))
	defer upstream.Close()

	adapter := NewOllamaAdapter()
	adapter.client = upstream.Client()
	_, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "", "llama3", "chat/completions", map[string]any{
		"model": "llama3",
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello "},
				map[string]any{"type": "text", "text": "world"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	messages := gotBody["messages"].([]any)
	content := messages[0].(map[string]any)["content"]
	if content != "Hello world" {
		t.Fatalf("flattened content = %v, want 'Hello world'", content)
	}
}

func TestOllamaAdapter_UpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
	}))
	defer upstream.Close()

	adapter := NewOllamaAdapter()
	adapter.client = upstream.Client()
	_, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "", "nope", "chat/completions", map[string]any{
		"model":    "nope",
		"messages": []any{map[string]any{"role": "user", "content": "hi"}},
	})
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected upstream error, got %v", err)
	}
}

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

func TestGeminiAdapter_ExecuteEndpoint(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"role":"model","parts":[{"text":"hey"}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":8,"candidatesTokenCount":3,"totalTokenCount":11}
		}`))
	}))
	defer upstream.Close()

	adapter := NewGeminiAdapter()
	adapter.client = upstream.Client()

	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "gemini-key",
		"gemini-2.5-flash", "chat/completions", map[string]any{
			"model": "gemini-2.5-flash",
			"messages": []any{
				map[string]any{"role": "system", "content": "sys"},
				map[string]any{"role": "user", "content": "hi"},
			},
		})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if gotPath != "/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("path = %q, want generateContent", gotPath)
	}
	// Request must be Gemini-shaped, not OpenAI chat.
	contents, ok := gotBody["contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("expected gemini contents, got %v", gotBody)
	}
	if gotBody["systemInstruction"] == nil {
		t.Fatal("expected systemInstruction mapping")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	choices, _ := resp.Body["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices: %v", resp.Body)
	}
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	if msg["content"] != "hey" {
		t.Fatalf("content = %v, want hey", msg["content"])
	}
	if choice["finish_reason"] != "stop" {
		t.Fatalf("finish_reason = %v, want stop", choice["finish_reason"])
	}
	if resp.Usage.TotalTokens != 11 || resp.UsageSource != usageparser.SourceUpstream {
		t.Fatalf("usage = %+v source = %s", resp.Usage, resp.UsageSource)
	}
}

func TestGeminiAdapter_URLSuffixes(t *testing.T) {
	cases := map[string]string{
		"https://api.example.com":         "https://api.example.com/v1beta/models/gemini-2.5-flash:generateContent",
		"https://api.example.com/v1":      "https://api.example.com/v1beta/models/gemini-2.5-flash:generateContent",
		"https://api.example.com/v1beta":  "https://api.example.com/v1beta/models/gemini-2.5-flash:generateContent",
		"https://api.example.com/v1beta/": "https://api.example.com/v1beta/models/gemini-2.5-flash:generateContent",
	}
	for base, want := range cases {
		if got := geminiGenerateURL(base, "gemini-2.5-flash"); got != want {
			t.Errorf("geminiGenerateURL(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestGeminiAdapter_RejectsUnsupportedEndpoints(t *testing.T) {
	adapter := NewGeminiAdapter()
	if _, err := adapter.ExecuteEndpointRaw(context.Background(), "u", "k", "m", "audio/speech", nil); err == nil {
		t.Fatal("expected raw endpoint error")
	}
	if _, err := adapter.ExecuteEndpointMultipart(context.Background(), "u", "k", "m", "audio/transcriptions", nil, nil); err == nil {
		t.Fatal("expected multipart endpoint error")
	}
}

func TestNewExecutorForConfig(t *testing.T) {
	gemini := NewExecutorForConfig(map[string]any{"upstream_format": "gemini"})
	if _, ok := gemini.(*GeminiAdapter); !ok {
		t.Fatalf("expected GeminiAdapter for upstream_format=gemini, got %T", gemini)
	}
	geminiUpper := NewExecutorForConfig(map[string]any{"upstream_format": "GEMINI"})
	if _, ok := geminiUpper.(*GeminiAdapter); !ok {
		t.Fatalf("expected GeminiAdapter for case-insensitive format, got %T", geminiUpper)
	}
	anthropic := NewExecutorForConfig(map[string]any{"upstream_format": "anthropic"})
	if _, ok := anthropic.(*AnthropicAdapter); !ok {
		t.Fatalf("expected AnthropicAdapter for upstream_format=anthropic, got %T", anthropic)
	}
	ollama := NewExecutorForConfig(map[string]any{"upstream_format": "ollama"})
	if _, ok := ollama.(*OllamaAdapter); !ok {
		t.Fatalf("expected OllamaAdapter for upstream_format=ollama, got %T", ollama)
	}
	openai := NewExecutorForConfig(map[string]any{"upstream_format": "openai"})
	if _, ok := openai.(*OpenAICompatAdapter); !ok {
		t.Fatalf("expected OpenAICompatAdapter default, got %T", openai)
	}
	defaultAdapter := NewExecutorForConfig(nil)
	if _, ok := defaultAdapter.(*OpenAICompatAdapter); !ok {
		t.Fatalf("expected OpenAICompatAdapter for nil config, got %T", defaultAdapter)
	}
}

func TestGeminiAdapter_ConvertsToolCalls(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"get_weather","args":{"city":"Shanghai"}}}]},"finishReason":"STOP"}],
			"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}
		}`))
	}))
	defer upstream.Close()

	adapter := NewGeminiAdapter()
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k", "gemini-2.5-flash", "chat/completions", map[string]any{
		"model":    "gemini-2.5-flash",
		"messages": []any{map[string]any{"role": "user", "content": "weather?"}},
	})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	choices := resp.Body["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	toolCalls := msg["tool_calls"].([]any)
	if len(toolCalls) != 1 {
		t.Fatalf("expected tool_calls, got %v", msg)
	}
	fn := toolCalls[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "get_weather" || !strings.Contains(fn["arguments"].(string), "Shanghai") {
		t.Fatalf("tool call mapping: %v", fn)
	}
}

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

func TestCustomAdapter_TemplateOverride(t *testing.T) {
	var gotPath, gotAuth, gotCustom string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotAuth = r.Header.Get("Authorization")
		gotCustom = r.Header.Get("X-Custom")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"custom-1","choices":[{"index":0,"message":{"role":"assistant","content":"hey custom"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	defer upstream.Close()

	adapter := NewCustomChannelAdapter(map[string]any{
		"custom_override": map[string]any{
			"method": "POST",
			"url":    upstream.URL + "/api/{model}/run?key={api_key}",
			"headers": map[string]any{
				"Authorization": "Token {api_key}",
				"X-Custom":      "yes",
			},
			"body": `{"model":"{model}","payload":{request_body}}`,
		},
	})
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "sk-secret",
		"my-model", "chat/completions", map[string]any{
			"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if !strings.HasPrefix(gotPath, "/api/my-model/run?key=sk-secret") {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Token sk-secret" || gotCustom != "yes" {
		t.Fatalf("headers auth=%q custom=%q", gotAuth, gotCustom)
	}
	if gotBody["model"] != "my-model" {
		t.Fatalf("body model = %v", gotBody["model"])
	}
	payload, ok := gotBody["payload"].(map[string]any)
	if !ok || payload["messages"] == nil {
		t.Fatalf("body payload missing (request_body token): %v", gotBody)
	}
	if resp.Usage.TotalTokens != 3 || resp.UsageSource != usageparser.SourceUpstream {
		t.Fatalf("usage = %+v source = %s", resp.Usage, resp.UsageSource)
	}
}

func TestCustomAdapter_FallsBackToStandardURL(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{}}`))
	}))
	defer upstream.Close()

	adapter := NewCustomChannelAdapter(map[string]any{})
	adapter.client = upstream.Client()
	if _, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k", "m", "chat/completions", map[string]any{}); err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
}

func TestCustomAdapter_NonJSONResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not json"))
	}))
	defer upstream.Close()

	adapter := NewCustomChannelAdapter(map[string]any{"custom_override": map[string]any{"url": upstream.URL + "/raw"}})
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k", "m", "chat/completions", map[string]any{})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if resp.Body["raw"] != "not json" {
		t.Fatalf("raw body = %v", resp.Body)
	}
	if resp.UsageSource != usageparser.SourceEstimated {
		t.Fatalf("expected estimated usage for non-OpenAI response, got %s", resp.UsageSource)
	}
}

func TestCustomExecutorSelection(t *testing.T) {
	adapter := NewExecutorForConfig(map[string]any{"upstream_format": "custom"})
	if _, ok := adapter.(*CustomChannelAdapter); !ok {
		t.Fatalf("expected CustomChannelAdapter, got %T", adapter)
	}
}

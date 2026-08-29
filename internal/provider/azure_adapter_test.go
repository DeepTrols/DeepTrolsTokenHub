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

func TestAzureAdapter_ExecuteEndpoint(t *testing.T) {
	var gotPath string
	var gotHeader http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-azure","object":"chat.completion","model":"gpt-4o",
			"choices":[{"index":0,"message":{"role":"assistant","content":"hey from azure"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":8,"completion_tokens":3,"total_tokens":11}
		}`))
	}))
	defer upstream.Close()

	adapter := NewAzureAdapter()
	adapter.client = upstream.Client()
	resp, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "azure-key",
		"gpt-4o", "chat/completions", map[string]any{
			"model":    "gpt-4o",
			"messages": []any{map[string]any{"role": "user", "content": "hi"}},
		})
	if err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if gotPath != "/openai/deployments/gpt-4o/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotHeader.Get("api-key") != "azure-key" {
		t.Fatalf("api-key header = %q", gotHeader.Get("api-key"))
	}
	choices := resp.Body["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["content"] != "hey from azure" {
		t.Fatalf("content = %v", msg["content"])
	}
	if resp.Usage.TotalTokens != 11 || resp.UsageSource != usageparser.SourceUpstream {
		t.Fatalf("usage = %+v source = %s", resp.Usage, resp.UsageSource)
	}
}

func TestAzureAdapter_APIVersionAndLegacyDeployment(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{}}`))
	}))
	defer upstream.Close()

	adapter := NewAzureAdapterWithConfig(map[string]any{
		"azure_api_version":       "2024-02-01",
		"azure_legacy_deployment": "true",
	})
	adapter.client = upstream.Client()
	if _, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k",
		"gpt-4.0-preview", "chat/completions", map[string]any{"model": "gpt-4.0-preview"}); err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if gotPath != "/openai/deployments/gpt-40-preview/chat/completions?api-version=2024-02-01" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestAzureAdapter_DefaultAPIVersion(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{}}`))
	}))
	defer upstream.Close()

	adapter := NewAzureAdapter()
	adapter.client = upstream.Client()
	if _, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k", "gpt-4o", "chat/completions", map[string]any{}); err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if !strings.Contains(gotPath, "api-version="+azureDefaultAPIVersion) {
		t.Fatalf("path = %q, want default api-version", gotPath)
	}
}

func TestAzureAdapter_RejectsUnsupportedEndpoints(t *testing.T) {
	adapter := NewAzureAdapter()
	if _, err := adapter.ExecuteEndpointRaw(context.Background(), "u", "k", "m", "audio/speech", nil); err == nil {
		t.Fatal("expected raw endpoint error")
	}
}

func TestAzureAdapter_ConfigBoolLike(t *testing.T) {
	if !configBoolLike(map[string]any{"x": true}, "x") {
		t.Fatal("native bool true should parse")
	}
	if !configBoolLike(map[string]any{"x": "true"}, "x") {
		t.Fatal("string true should parse")
	}
	if configBoolLike(map[string]any{"x": "false"}, "x") {
		t.Fatal("string false should not parse")
	}
	if configBoolLike(map[string]any{}, "x") {
		t.Fatal("missing key should be false")
	}
}

func TestAzureExecutorSelection(t *testing.T) {
	adapter := NewExecutorForConfig(map[string]any{"upstream_format": "azure", "azure_api_version": "2024-02-01"})
	az, ok := adapter.(*AzureAdapter)
	if !ok {
		t.Fatalf("expected AzureAdapter, got %T", adapter)
	}
	if az.apiVersion != "2024-02-01" {
		t.Fatalf("apiVersion = %q", az.apiVersion)
	}
}

func TestAzureAdapter_JSONBodyUnchanged(t *testing.T) {
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{}}`))
	}))
	defer upstream.Close()

	adapter := NewAzureAdapter()
	adapter.client = upstream.Client()
	if _, err := adapter.ExecuteEndpoint(context.Background(), upstream.URL, "k", "gpt-4o", "chat/completions", map[string]any{
		"model": "gpt-4o", "messages": []any{map[string]any{"role": "user", "content": "hi"}},
	}); err != nil {
		t.Fatalf("ExecuteEndpoint: %v", err)
	}
	if gotBody["model"] != "gpt-4o" {
		t.Fatalf("body model = %v", gotBody["model"])
	}
}

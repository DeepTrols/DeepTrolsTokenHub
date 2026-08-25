package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gw "github.com/deeptrols/api/internal/service/gateway"
)

func TestOpenAICompatAdapter_ExecuteParsesUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["model"] != "deepseek-chat" {
			t.Errorf("model = %v, want deepseek-chat", body["model"])
		}
		w.Header().Set("x-request-id", "upstream-req-1")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-1",
			"usage": map[string]any{
				"prompt_tokens":           float64(100),
				"completion_tokens":       float64(50),
				"prompt_cache_hit_tokens": float64(40),
				"total_tokens":            float64(150),
			},
		})
	}))
	defer upstream.Close()

	adapter := NewOpenAICompatAdapter()
	resp, err := adapter.Execute(context.Background(), upstream.URL,
		"sk-test", "deepseek-chat", map[string]any{"messages": []any{}})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.ProviderReqID != "chatcmpl-1" {
		t.Errorf("provider req id = %q", resp.ProviderReqID)
	}
	if resp.Usage.InputTokens != 100 || resp.Usage.OutputTokens != 50 || resp.Usage.CacheReadTokens != 40 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.DurationMs < 0 {
		t.Error("duration must be non-negative")
	}
}

func TestOpenAICompatAdapter_AppliesCustomHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Custom-Header"); got != "v1" {
			t.Errorf("X-Custom-Header = %q, want v1", got)
		}
		if got := r.Header.Get("X-Provider-Id"); got != "my-gateway" {
			t.Errorf("X-Provider-Id = %q, want my-gateway", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("authorization = %q, want Bearer sk-test (default must remain)", got)
		}
		w.Header().Set("x-request-id", "upstream-req-2")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-2",
			"usage": map[string]any{
				"prompt_tokens":     float64(10),
				"completion_tokens": float64(5),
				"total_tokens":      float64(15),
			},
		})
	}))
	defer upstream.Close()

	adapter := NewOpenAICompatAdapter()
	extra := map[string]string{
		"X-Custom-Header": "v1",
		"X-Provider-Id":   "my-gateway",
	}
	resp, err := adapter.Execute(context.Background(), upstream.URL,
		"sk-test", "deepseek-chat", map[string]any{"messages": []any{}}, extra)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.ProviderReqID != "chatcmpl-2" {
		t.Errorf("provider req id = %q", resp.ProviderReqID)
	}
}

func TestOpenAICompatAdapter_ExecuteEndpointRaw(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("fake-audio-bytes"))
	}))
	defer upstream.Close()

	adapter := NewOpenAICompatAdapter()
	raw, err := adapter.ExecuteEndpointRaw(context.Background(), upstream.URL,
		"sk-test", "tts-1", "audio/speech", map[string]any{"input": "你好"})
	if err != nil {
		t.Fatalf("execute raw: %v", err)
	}
	if string(raw.Body) != "fake-audio-bytes" || raw.ContentType != "audio/mpeg" {
		t.Errorf("raw = %q / %q", raw.Body, raw.ContentType)
	}
}

func TestOpenAICompatAdapter_ExecuteEndpointMultipart(t *testing.T) {
	t.Run("json response", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/audio/transcriptions" {
				t.Errorf("path = %s, want /v1/audio/transcriptions", r.URL.Path)
			}
			if got := r.Header.Get("X-Custom-Header"); got != "v1" {
				t.Errorf("X-Custom-Header = %q, want v1", got)
			}
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if got := r.FormValue("model"); got != "whisper-1" {
				t.Errorf("model = %q, want whisper-1", got)
			}
			if got := r.FormValue("language"); got != "zh" {
				t.Errorf("language = %q, want zh", got)
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("form file: %v", err)
			}
			content, _ := io.ReadAll(f)
			_ = f.Close()
			if string(content) != "fake-audio" {
				t.Errorf("file content = %q, want fake-audio", string(content))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"text": "你好"})
		}))
		defer upstream.Close()

		adapter := NewOpenAICompatAdapter()
		resp, err := adapter.ExecuteEndpointMultipart(context.Background(), upstream.URL,
			"sk-test", "whisper-1", "audio/transcriptions",
			map[string]any{"language": "zh"},
			map[string]gw.MultipartFile{"file": {FileName: "a.mp3", ContentType: "audio/mpeg", Content: []byte("fake-audio")}},
			map[string]string{"X-Custom-Header": "v1"},
		)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if resp.Body["text"] != "你好" {
			t.Errorf("text = %v, want 你好", resp.Body["text"])
		}
	})

	t.Run("plain text response", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("transcribed plain text"))
		}))
		defer upstream.Close()

		adapter := NewOpenAICompatAdapter()
		resp, err := adapter.ExecuteEndpointMultipart(context.Background(), upstream.URL,
			"sk-test", "whisper-1", "audio/transcriptions",
			nil,
			map[string]gw.MultipartFile{"file": {FileName: "a.mp3", ContentType: "audio/mpeg", Content: []byte("fake-audio")}},
		)
		if err != nil {
			t.Fatalf("execute: %v", err)
		}
		if resp.Body["text"] != "transcribed plain text" {
			t.Errorf("text = %v, want transcribed plain text", resp.Body["text"])
		}
	})
}

func TestBuildUpstreamRequest(t *testing.T) {
	reqBytes, url, err := buildUpstreamRequest("https://api.deepseek.com/v1", "deepseek-chat", "chat/completions", map[string]any{"temperature": 0.2})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if url != "https://api.deepseek.com/v1/chat/completions" {
		t.Errorf("url = %s", url)
	}
	var body map[string]any
	if err := json.Unmarshal(reqBytes, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["model"] != "deepseek-chat" || body["temperature"] != 0.2 {
		t.Errorf("body = %#v", body)
	}
}

func TestInferCategory(t *testing.T) {
	tmpl, _ := Lookup("qwen")
	cases := map[string]string{
		"deepseek-chat":                  "chat",
		"text-embedding-v3":              "embedding",
		"bge-m3":                         "embedding",
		"qwen-vl-plus":                   "chat",
		"wanx2.1-t2i-turbo":              "image",
		"cogview-3-flash":                "image",
		"tts-1":                          "audio",
		"cosyvoice-v1":                   "audio",
		"seedance-1-0-pro":               "video",
		"doubao-seedance-1-0-pro-250528": "video",
	}
	for modelID, want := range cases {
		if got := InferCategory(modelID, tmpl); got != want {
			t.Errorf("InferCategory(%q) = %q, want %q", modelID, got, want)
		}
	}
}

func TestInferCategoryEmptyTemplateStillChat(t *testing.T) {
	if got := InferCategory("anything-model", Template{}); got != "chat" {
		t.Errorf("InferCategory with empty template = %q, want chat", got)
	}
}

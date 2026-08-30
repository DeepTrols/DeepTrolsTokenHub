// Command echo_upstream is an OpenAI-compatible echo upstream used by the
// Playwright E2E smoke test. It records request headers to echo_headers.log
// and returns canned model/chat responses so the smoke test never needs
// external network access.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", modelsHandler)
	mux.HandleFunc("/models", modelsHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/chat/completions", chatHandler)
	mux.HandleFunc("/v1beta/models/", geminiHandler)
	mux.HandleFunc("/v1/messages", messagesHandler)
	mux.HandleFunc("/api/chat", ollamaChatHandler)
	mux.HandleFunc("/openai/deployments/", azureChatHandler)
	mux.HandleFunc("/custom/", customHandler)
	mux.HandleFunc("/v1/audio/transcriptions", transcriptionHandler)
	mux.HandleFunc("/v1/images/edits", imagesEditsHandler)
	mux.HandleFunc("/v1/videos/generations", videoGenerationsHandler)
	addr := "127.0.0.1:8090"
	fmt.Printf("echo upstream listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// geminiHandler is a Gemini-native generateContent mock used by the
// upstream_format=gemini channel smoke test.
func geminiHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"candidates": []map[string]any{{
			"content":      map[string]any{"role": "model", "parts": []map[string]any{{"text": "echo from gemini"}}},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount":     1,
			"candidatesTokenCount": 1,
			"totalTokenCount":      2,
		},
	})
}

// messagesHandler is an Anthropic Messages API mock used by the
// upstream_format=anthropic channel smoke test.
func messagesHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "msg_echo", "type": "message", "role": "assistant", "model": "claude-sonnet-4",
		"content":     []map[string]any{{"type": "text", "text": "echo from claude"}},
		"stop_reason": "end_turn",
		"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
	})
}

// ollamaChatHandler is an Ollama /api/chat mock used by the
// upstream_format=ollama channel smoke test.
func ollamaChatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"model": "llama3", "created_at": "2026-08-29T00:00:00Z",
		"message": map[string]any{"role": "assistant", "content": "echo from ollama"},
		"done":    true,
		"prompt_eval_count": 2, "eval_count": 3,
	})
}

// azureChatHandler is an Azure OpenAI deployment mock used by the
// upstream_format=azure channel smoke test.
func azureChatHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "chatcmpl-azure", "object": "chat.completion", "model": "gpt-4o",
		"choices": []map[string]any{{
			"index": 0, "message": map[string]any{"role": "assistant", "content": "echo from azure"},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
}

// customHandler is an arbitrary custom-channel mock: it echoes the request
// path (reflecting {model}/{api_key} placeholders) and returns an OpenAI-
// shaped JSON response so billing continues to work.
func customHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "custom-echo", "object": "chat.completion", "model": r.URL.Path,
		"choices": []map[string]any{{
			"index": 0, "message": map[string]any{"role": "assistant", "content": "echo from custom " + r.URL.Path},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		// DeepSeek 官方目录只有 V4 三模型（deepseek-chat 已于 2026-07-24 停用）。
		"data": []map[string]any{
			{"id": "deepseek-v4-flash"},
			{"id": "deepseek-v4-pro"},
			{"id": "deepseek-v4-flash-vision-exp"},
		},
	})
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	f, err := os.OpenFile("echo_headers.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		keys := make([]string, 0, len(r.Header))
		for k := range r.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		fmt.Fprintf(&b, "%s headers:\n", time.Now().Format(time.RFC3339))
		for _, k := range keys {
			fmt.Fprintf(&b, "  %s: %s\n", k, r.Header.Get(k))
		}
		_, _ = f.WriteString(b.String())
		_ = f.Close()
	}

	// SSE streaming mode (used by /v1/messages and chat streaming smoke tests):
	// emit role → content deltas → finish+usage chunks, then [DONE].
	rawBody, _ := io.ReadAll(r.Body)
	if f, err := os.OpenFile("echo_headers.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		fmt.Fprintf(f, "  body: %s\n", string(rawBody))
		_ = f.Close()
	}
	var reqBody map[string]any
	if err := json.Unmarshal(rawBody, &reqBody); err == nil {
		if stream, _ := reqBody["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, _ := w.(http.Flusher)
			chunks := []string{
				`{"id":"chatcmpl-echo","object":"chat.completion.chunk","model":"echo","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
				`{"id":"chatcmpl-echo","object":"chat.completion.chunk","model":"echo","choices":[{"index":0,"delta":{"content":"echo"},"finish_reason":null}]}`,
				`{"id":"chatcmpl-echo","object":"chat.completion.chunk","model":"echo","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			}
			for _, c := range chunks {
				fmt.Fprintf(w, "data: %s\n\n", c)
				flusher.Flush()
			}
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": "echo-1",
		"choices": []map[string]any{{
			"message":       map[string]any{"role": "assistant", "content": "echo"},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{
			"prompt_tokens":     1,
			"completion_tokens": 1,
			"total_tokens":      2,
		},
	})
}

func transcriptionHandler(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	if _, _, err := r.FormFile("file"); err != nil {
		http.Error(w, `{"error":"file required"}`, http.StatusBadRequest)
		return
	}
	if r.FormValue("model") == "" {
		http.Error(w, `{"error":"model required"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"text": "你好"})
}

func imagesEditsHandler(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	if _, _, err := r.FormFile("image"); err != nil {
		http.Error(w, `{"error":"image required"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"created": 1,
		"data":    []map[string]any{{"url": "https://img.example.com/e2e.png"}},
	})
}

func videoGenerationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST only"}`, http.StatusMethodNotAllowed)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body["model"] == "" {
		http.Error(w, `{"error":"model required"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": "upstream-video-1"})
}

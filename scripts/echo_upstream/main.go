// Command echo_upstream is an OpenAI-compatible echo upstream used by the
// Playwright E2E smoke test. It records request headers to echo_headers.log
// and returns canned model/chat responses so the smoke test never needs
// external network access.
package main

import (
	"encoding/json"
	"fmt"
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
	mux.HandleFunc("/v1/chat/completions", chatHandler)
	mux.HandleFunc("/v1/audio/transcriptions", transcriptionHandler)
	mux.HandleFunc("/v1/images/edits", imagesEditsHandler)
	addr := "127.0.0.1:8090"
	fmt.Printf("echo upstream listening on %s\n", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func modelsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": []map[string]any{{"id": "deepseek-chat"}},
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

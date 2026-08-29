package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
)

func TestCountAnthropicTokens_Estimates(t *testing.T) {
	// 20 CJK chars = 20 tokens.
	if got := countAnthropicTokens(map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "你好世界你好世界你好世界你好世界你好世界"},
		},
	}); got != 20 {
		t.Fatalf("cjk content tokens = %d, want 20", got)
	}

	// System string + text blocks + tools all count.
	total := countAnthropicTokens(map[string]any{
		"system": "You are helpful",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "hello"},
					map[string]any{"type": "image", "source": map[string]any{"type": "base64"}},
				},
			},
		},
		"tools": []any{
			map[string]any{"name": "get_weather", "description": "weather lookup"},
		},
	})
	if total <= 0 {
		t.Fatalf("expected positive estimate, got %d", total)
	}

	// Empty payload still returns at least 1 (Anthropic never returns 0).
	if got := countAnthropicTokens(map[string]any{}); got != 1 {
		t.Fatalf("empty tokens = %d, want 1", got)
	}
}

func TestHandleCountTokens_ReturnsEstimate(t *testing.T) {
	a := &app.App{}
	body, _ := json.Marshal(map[string]any{
		"model": "claude-sonnet-4",
		"messages": []any{
			map[string]any{"role": "user", "content": "ping"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	w := httptest.NewRecorder()
	HandleCountTokens(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		InputTokens int64 `json:"input_tokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.InputTokens <= 0 {
		t.Fatalf("input_tokens = %d, want > 0", resp.InputTokens)
	}
}

func TestHandleCountTokens_InvalidBody(t *testing.T) {
	a := &app.App{}
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader([]byte("not-json")))
	w := httptest.NewRecorder()
	HandleCountTokens(a).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

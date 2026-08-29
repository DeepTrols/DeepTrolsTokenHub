package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
)

// HandleCountTokens implements POST /v1/messages/count_tokens (Anthropic
// compatibility, new-api parity): a free token-count estimate for a messages
// payload. No routing, no billing — the endpoint only estimates input tokens
// from message/system text (and tool definitions).
func HandleCountTokens(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{
			"input_tokens": countAnthropicTokens(body),
		})
	}
}

// countAnthropicTokens estimates the input token count for an Anthropic
// messages payload: string or text-block content in system + messages, plus
// serialized tool definitions.
func countAnthropicTokens(body map[string]any) int64 {
	var total int64
	addText := func(v any) {
		switch s := v.(type) {
		case string:
			total += usageparser.EstimateTextTokens(s)
		case []any:
			for _, block := range s {
				if b, ok := block.(map[string]any); ok {
					if txt, ok := b["text"].(string); ok {
						total += usageparser.EstimateTextTokens(txt)
					}
				}
			}
		}
	}
	if sys, ok := body["system"]; ok {
		addText(sys)
	}
	if msgs, ok := body["messages"].([]any); ok {
		for _, m := range msgs {
			if mm, ok := m.(map[string]any); ok {
				addText(mm["content"])
			}
		}
	}
	if tools, ok := body["tools"].([]any); ok {
		for _, tl := range tools {
			if raw, err := json.Marshal(tl); err == nil {
				total += usageparser.EstimateTextTokens(string(raw))
			}
		}
	}
	if total <= 0 {
		total = 1
	}
	return total
}

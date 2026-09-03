package gateway

import (
	"errors"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
)

// HandleCompletions implements POST /v1/completions (legacy OpenAI completions,
// non-streaming passthrough with billing from upstream usage).
func HandleCompletions(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleForwardedEndpoint(w, r, application, "completions", "chat",
			validateCompletionsRequest, estimateCompletionsUsage)
	}
}

func validateCompletionsRequest(body map[string]any) error {
	if body["model"] == nil || body["model"] == "" {
		return errors.New("model is required")
	}
	// A completions request without a usable prompt must be rejected up
	// front (TH-P05-12 / AC-03): an empty prompt previously produced an
	// empty usage estimate and a silent minimum hold.
	if _, ok := completionsPromptTokens(body["prompt"]); !ok {
		return errors.New("prompt is required")
	}
	return nil
}

// completionsPromptTokens counts input tokens for a legacy completions
// prompt: a string, a batch of strings, or token-id arrays (one id equals
// one token). ok=false reports a missing/unusable prompt so callers can
// distinguish "nothing to bill" from "cannot estimate".
func completionsPromptTokens(raw any) (int64, bool) {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return 0, false
		}
		return int64(usageparser.EstimateTextTokens(v)), true
	case []any:
		if len(v) == 0 {
			return 0, false
		}
		var total int64
		for _, item := range v {
			switch it := item.(type) {
			case string:
				total += int64(usageparser.EstimateTextTokens(it))
			case []any:
				// Token-id array: one id IS one token (exact count).
				total += int64(len(it))
			case float64:
				// Flat token-id array ([123, 456]): each id is one token.
				total++
			}
		}
		return total, true
	}
	return 0, false
}

// estimateCompletionsUsage builds the maximum-exposure usage for a legacy
// completions request (TH-P05-12 / C-1): the prompt token estimate plus the
// declared maximum output (capped). This replaces the previous empty
// NormalizedUsage{}, which floored every completions hold at the silent
// minimum.
func estimateCompletionsUsage(body map[string]any) *usageparser.NormalizedUsage {
	input, ok := completionsPromptTokens(body["prompt"])
	if !ok || input <= 0 {
		// Validation guarantees a usable prompt; the floor protects the
		// boundary/settle fallbacks if the estimator is reached directly.
		input = 1
	}
	out, _ := maxHoldOutputTokens(body)
	return &usageparser.NormalizedUsage{
		InputTokens:  input,
		OutputTokens: out,
		TotalTokens:  input + out,
	}
}

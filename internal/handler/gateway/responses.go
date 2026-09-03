package gateway

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
)

// HandleResponses implements POST /v1/responses (OpenAI Responses API, non-streaming).
// It forwards to the routed upstream's /v1/responses endpoint and settles billing
// from the upstream usage (no format conversion yet).
func HandleResponses(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, modelName, ok := decodeForwardedRequest(w, r, application, validateResponsesRequest, estimateResponsesUsage)
		if !ok {
			return
		}
		// Chat-completions-only channels get the Responses request converted
		// to a chat call and converted back (new-api responses_via_chat).
		// Conversion drops multimodal parts before the upstream call, so the
		// chat hold based on the surviving text is correct there; the strict
		// gate below applies only to the direct-forward branch.
		if responsesViaChat(application, r, modelName) {
			HandleResponsesViaChat(w, r, application, modelName, body)
			return
		}
		// Direct forward (TH-P05-12 / AC-02): the hold must be built from
		// the strict polymorphic input estimate. When the input contains
		// content the estimator cannot bound, fail closed instead of holding
		// a silent minimum.
		strictUsage, priceable := estimateResponsesUsageStrict(body)
		if !priceable {
			log.Printf("gateway: responses direct forward fail-closed: unpriceable input content")
			writeError(w, http.StatusUnprocessableEntity, "pricing_incomplete", "Unable to price request reliably")
			return
		}
		handleForwardedEndpointExecution(w, r, application, "responses", "chat", modelName, body,
			func(map[string]any) *usageparser.NormalizedUsage { return strictUsage })
	}
}

func validateResponsesRequest(body map[string]any) error {
	if body["model"] == nil || body["model"] == "" {
		return errors.New("model is required")
	}
	return nil
}

// responsesHoldOutputTokens is the output bound for Responses holds: the
// declared max_output_tokens (capped at maxChargeOutputCap) wins; otherwise
// the shared max_completion_tokens / max_tokens chain and finally the
// documented fallback cap apply.
func responsesHoldOutputTokens(body map[string]any) int64 {
	if v, ok := declaredOutputTokens(body["max_output_tokens"]); ok {
		if v > maxChargeOutputCap {
			v = maxChargeOutputCap
		}
		return v
	}
	out, _ := maxHoldOutputTokens(body)
	return out
}

// estimateResponsesUsageStrict builds the maximum-exposure usage for a
// Responses request from its polymorphic input (TH-P05-12 / C-1): the
// instructions text plus every input item. ok=false reports content the
// estimator cannot bound (e.g. file parts), which the direct-forward path
// must reject instead of silently holding the minimum.
func estimateResponsesUsageStrict(body map[string]any) (*usageparser.NormalizedUsage, bool) {
	var input int64
	if instr, ok := body["instructions"].(string); ok {
		input += int64(usageparser.EstimateTextTokens(instr))
	}
	switch v := body["input"].(type) {
	case nil:
		// No input: instructions-only requests are legitimate.
	case string:
		input += int64(usageparser.EstimateTextTokens(v))
	case []any:
		for _, item := range v {
			n, ok := responsesItemTokens(item)
			if !ok {
				return nil, false
			}
			input += n
		}
	default:
		// Unknown input shape: cannot price reliably.
		return nil, false
	}
	if input <= 0 {
		input = 1
	}
	out := responsesHoldOutputTokens(body)
	return &usageparser.NormalizedUsage{
		InputTokens:  input,
		OutputTokens: out,
		TotalTokens:  input + out,
	}, true
}

// responsesItemTokens counts the billable input tokens of one Responses
// input item. Strings are estimated as text; item maps contribute their
// content (string or multimodal parts via the shared chat classifier) and
// any function-call arguments; unknown non-map shapes are billed by their
// JSON encoding rather than skipped. ok=false means the item carries content
// with no bounded token allowance.
func responsesItemTokens(item any) (int64, bool) {
	switch it := item.(type) {
	case string:
		return int64(usageparser.EstimateTextTokens(it)), true
	case map[string]any:
		var total int64
		switch c := it["content"].(type) {
		case string:
			total += int64(usageparser.EstimateTextTokens(c))
		case []any:
			for _, p := range c {
				n, ok := chatContentPartTokens(p)
				if !ok {
					return 0, false
				}
				total += n
			}
		}
		// Function-call items carry their arguments as billed input.
		if call, ok := it["call"].(map[string]any); ok {
			if args, ok := call["arguments"].(string); ok {
				total += int64(usageparser.EstimateTextTokens(args))
			}
		}
		return total, true
	default:
		raw, err := json.Marshal(item)
		if err != nil {
			return 0, false
		}
		return int64(usageparser.EstimateTextTokens(string(raw))), true
	}
}

// estimateResponsesUsage is the lenient estimate used by the shared decode
// path (API-key boundary checks). The reserve path uses the strict estimator
// and fails closed on unpriceable content instead (see HandleResponses).
func estimateResponsesUsage(body map[string]any) *usageparser.NormalizedUsage {
	if usage, ok := estimateResponsesUsageStrict(body); ok {
		return usage
	}
	out := responsesHoldOutputTokens(body)
	return &usageparser.NormalizedUsage{
		InputTokens:  1,
		OutputTokens: out,
		TotalTokens:  1 + out,
	}
}

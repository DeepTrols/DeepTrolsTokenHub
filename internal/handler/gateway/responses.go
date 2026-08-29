package gateway

import (
	"errors"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
)

// HandleResponses implements POST /v1/responses (OpenAI Responses API, non-streaming).
// It forwards to the routed upstream's /v1/responses endpoint and settles billing
// from the upstream usage (new-api Responses parity, no format conversion yet).
func HandleResponses(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, modelName, ok := decodeForwardedRequest(w, r, application, validateResponsesRequest, estimateResponsesUsage)
		if !ok {
			return
		}
		// Chat-completions-only channels get the Responses request converted
		// to a chat call and converted back (new-api responses_via_chat).
		if responsesViaChat(application, r, modelName) {
			HandleResponsesViaChat(w, r, application, modelName, body)
			return
		}
		handleForwardedEndpointExecution(w, r, application, "responses", "chat", modelName, body, estimateResponsesUsage)
	}
}

func validateResponsesRequest(body map[string]any) error {
	if body["model"] == nil || body["model"] == "" {
		return errors.New("model is required")
	}
	return nil
}

// estimateResponsesUsage returns a minimal pre-hold usage estimate. The actual
// settlement uses the upstream Responses usage parsed by the executor, so this
// only needs to keep the hold non-zero-safe (minHoldAmount fallback covers 0).
func estimateResponsesUsage(body map[string]any) *usageparser.NormalizedUsage {
	return &usageparser.NormalizedUsage{}
}

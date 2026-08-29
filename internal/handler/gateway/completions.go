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
	return nil
}

func estimateCompletionsUsage(body map[string]any) *usageparser.NormalizedUsage {
	return &usageparser.NormalizedUsage{}
}

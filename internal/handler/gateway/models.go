package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/handler/middleware"
	providerpkg "github.com/deeptrols/api/internal/provider"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// HandleListModels returns the list of models available to the authenticated key/tenant.
func HandleListModels(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Only GET is allowed",
					"type":    "method_not_allowed",
				},
			})
			return
		}

		keyIDStr, ok := r.Context().Value(middleware.CtxAPIKeyID).(string)
		if !ok || keyIDStr == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Missing API key in request context",
					"type":    "auth_error",
				},
			})
			return
		}

		keyID, err := uuid.Parse(keyIDStr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Invalid API key identifier",
					"type":    "auth_error",
				},
			})
			return
		}

		apiKey, err := application.APIKeys.FindByID(r.Context(), keyID)
		if err != nil || apiKey == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": fmt.Sprintf("API key not found: %v", err),
					"type":    "auth_error",
				},
			})
			return
		}

		domainModels, err := application.Models.ListActive(r.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{
					"message": "Failed to list models",
					"type":    "server_error",
				},
			})
			return
		}

		allowSet := make(map[string]bool, len(apiKey.AllowedModels))
		hasAllowlist := len(apiKey.AllowedModels) > 0
		if hasAllowlist {
			for _, m := range apiKey.AllowedModels {
				allowSet[m] = true
			}
		}

		data := make([]map[string]any, 0)
		for _, m := range domainModels {
			if !providerpkg.IsDomesticProvider(m.Provider) {
				continue
			}
			if hasAllowlist && !allowSet[m.Code] {
				continue
			}
			data = append(data, map[string]any{
				"id":       m.Code,
				"object":   "model",
				"created":  m.CreatedAt.Unix(),
				"owned_by": m.Provider,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data":   data,
		})
	}
}

// HandleRetrieveModel implements GET /v1/models/{model}: return a single active
// model by code (OpenAI-compatible retrieve semantics).
func HandleRetrieveModel(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "model")
		m, err := application.Models.FindByCode(r.Context(), code)
		if err != nil || !providerpkg.IsDomesticProvider(m.Provider) {
			writeError(w, http.StatusNotFound, "model_not_found", "No such model")
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{
			"id":       m.Code,
			"object":   "model",
			"created":  m.CreatedAt.Unix(),
			"owned_by": m.Provider,
		})
	}
}

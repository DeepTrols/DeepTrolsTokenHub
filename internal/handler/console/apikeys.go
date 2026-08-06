package console

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/encrypt"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/pkg/keyhash"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const apiKeyPrefix = "dt-sk-"

type apiKeyResponse struct {
	ID              string   `json:"id"`
	KeyPrefix       string   `json:"key_prefix"`
	Name            string   `json:"name"`
	MaskedKey       string   `json:"masked_key"`
	Status          string   `json:"status"`
	AllowedModels   []string `json:"allowed_models,omitempty"`
	SourceWhitelist []string `json:"source_whitelist,omitempty"`
	CumulativeLimit string   `json:"cumulative_limit,omitempty"`
	WeeklyLimit     string   `json:"weekly_limit,omitempty"`
	MonthlyLimit    string   `json:"monthly_limit,omitempty"`
	OverLimitAction string   `json:"over_limit_action,omitempty"`
	LastUsedAt      string   `json:"last_used_at,omitempty"`
	Last7dActive    bool     `json:"last_7d_active"`
	CreatedAt       string   `json:"created_at"`
}

type createAPIKeyRequest struct {
	Name            string   `json:"name"`
	AllowedModels   []string `json:"allowed_models,omitempty"`
	SourceWhitelist []string `json:"source_whitelist,omitempty"`
	MonthlyLimit    string   `json:"monthly_limit,omitempty"`
	WeeklyLimit     string   `json:"weekly_limit,omitempty"`
	CumulativeLimit string   `json:"cumulative_limit,omitempty"`
	OverLimitAction string   `json:"over_limit_action,omitempty"`
}

type createAPIKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Plaintext string `json:"plaintext"`
	KeyPrefix string `json:"key_prefix"`
	MaskedKey string `json:"masked_key"`
	Warning   string `json:"warning"`
}

// HandleListAPIKeys returns all API keys for the authenticated user.
func HandleListAPIKeys(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		keys, err := a.APIKeys.ListByUser(r.Context(), userID, nil)
		if err != nil {
			log.Printf("HandleListAPIKeys: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list API keys"})
			return
		}

		response := make([]apiKeyResponse, 0, len(keys))
		for _, k := range keys {
			if k.Status == domain.APIKeyStatusRevoked {
				continue
			}
			item := apiKeyResponse{
				ID:              k.ID.String(),
				Name:            k.Name,
				MaskedKey:       k.MaskedKey,
				Status:          string(k.Status),
				AllowedModels:   k.AllowedModels,
				OverLimitAction: string(k.OverLimitAction),
				Last7dActive:    k.Last7dActive,
				CreatedAt:       k.CreatedAt.Format(time.RFC3339),
			}
			if k.KeyPrefix != "" {
				item.KeyPrefix = k.KeyPrefix
			}
			if len(k.SourceWhitelist) > 0 {
				item.SourceWhitelist = k.SourceWhitelist
			}
			if !k.CumulativeLimit.IsZero() {
				item.CumulativeLimit = k.CumulativeLimit.String()
			}
			if !k.WeeklyLimit.IsZero() {
				item.WeeklyLimit = k.WeeklyLimit.String()
			}
			if !k.MonthlyLimit.IsZero() {
				item.MonthlyLimit = k.MonthlyLimit.String()
			}
			if k.LastUsedAt != nil {
				item.LastUsedAt = k.LastUsedAt.Format(time.RFC3339)
			}
			response = append(response, item)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": len(response),
		})
	}
}

// HandleCreateAPIKey creates a new API key for the authenticated user.
func HandleCreateAPIKey(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		var req createAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
			return
		}

		// Generate cryptographically random key material.
		plaintext, keyHash, maskedKey, err := generateAPIKeyMaterial(a.Config.Encryption.Key)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate API key"})
			return
		}

		// Encrypt plaintext so it can be retrieved later.
		encryptedKey, err := encryptPlaintext(a, plaintext)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to encrypt API key"})
			return
		}

		key := domain.NewAPIKey(userID, nil, apiKeyPrefix, keyHash, maskedKey, req.Name)
		key.EncryptedKey = encryptedKey

		// Set optional fields.
		if len(req.AllowedModels) > 0 {
			key.AllowedModels = req.AllowedModels
		}
		if len(req.SourceWhitelist) > 0 {
			key.SourceWhitelist = req.SourceWhitelist
		}
		if req.MonthlyLimit != "" {
			limit, err := decimal.NewFromString(req.MonthlyLimit)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid monthly_limit"})
				return
			}
			key.MonthlyLimit = limit
		}
		if req.WeeklyLimit != "" {
			limit, err := decimal.NewFromString(req.WeeklyLimit)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid weekly_limit"})
				return
			}
			key.WeeklyLimit = limit
		}
		if req.CumulativeLimit != "" {
			limit, err := decimal.NewFromString(req.CumulativeLimit)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid cumulative_limit"})
				return
			}
			key.CumulativeLimit = limit
		}
		if req.OverLimitAction != "" {
			action := domain.OverLimitAction(req.OverLimitAction)
			if action != domain.OverLimitBlock && action != domain.OverLimitWarn {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid over_limit_action, must be 'block' or 'warn'"})
				return
			}
			key.OverLimitAction = action
		}

		if err := a.APIKeys.Create(r.Context(), &key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create API key"})
			return
		}

		writeJSON(w, http.StatusCreated, createAPIKeyResponse{
			ID:        key.ID.String(),
			Name:      key.Name,
			Plaintext: plaintext,
			KeyPrefix: apiKeyPrefix,
			MaskedKey: maskedKey,
			Warning:   "Store this key securely. It will not be shown again.",
		})
	}
}

// HandleUpdateAPIKey updates an existing API key owned by the authenticated user.
func HandleUpdateAPIKey(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		keyID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid key ID"})
			return
		}

		key, err := a.APIKeys.FindByID(r.Context(), keyID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
			return
		}

		if key.UserID != userID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
			return
		}

		var req struct {
			Name            *string  `json:"name,omitempty"`
			AllowedModels   []string `json:"allowed_models,omitempty"`
			SourceWhitelist []string `json:"source_whitelist,omitempty"`
			MonthlyLimit    *string  `json:"monthly_limit,omitempty"`
			WeeklyLimit     *string  `json:"weekly_limit,omitempty"`
			CumulativeLimit *string  `json:"cumulative_limit,omitempty"`
			OverLimitAction *string  `json:"over_limit_action,omitempty"`
			Status          *string  `json:"status,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Name != nil {
			key.Name = *req.Name
		}
		if len(req.AllowedModels) > 0 {
			key.AllowedModels = req.AllowedModels
		}
		if len(req.SourceWhitelist) > 0 {
			key.SourceWhitelist = req.SourceWhitelist
		}
		if req.MonthlyLimit != nil {
			if *req.MonthlyLimit == "" {
				key.MonthlyLimit = decimal.Zero
			} else {
				limit, err := decimal.NewFromString(*req.MonthlyLimit)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid monthly_limit"})
					return
				}
				key.MonthlyLimit = limit
			}
		}
		if req.WeeklyLimit != nil {
			if *req.WeeklyLimit == "" {
				key.WeeklyLimit = decimal.Zero
			} else {
				limit, err := decimal.NewFromString(*req.WeeklyLimit)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid weekly_limit"})
					return
				}
				key.WeeklyLimit = limit
			}
		}
		if req.CumulativeLimit != nil {
			if *req.CumulativeLimit == "" {
				key.CumulativeLimit = decimal.Zero
			} else {
				limit, err := decimal.NewFromString(*req.CumulativeLimit)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid cumulative_limit"})
					return
				}
				key.CumulativeLimit = limit
			}
		}
		if req.OverLimitAction != nil {
			if *req.OverLimitAction == "" {
				key.OverLimitAction = domain.OverLimitBlock
			} else {
				action := domain.OverLimitAction(*req.OverLimitAction)
				if action != domain.OverLimitBlock && action != domain.OverLimitWarn {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid over_limit_action, must be 'block' or 'warn'"})
					return
				}
				key.OverLimitAction = action
			}
		}
		if req.Status != nil {
			status := domain.APIKeyStatus(*req.Status)
			switch status {
			case domain.APIKeyStatusActive, domain.APIKeyStatusDisabled:
				key.Status = status
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status, must be 'active' or 'disabled'"})
				return
			}
		}
		key.UpdatedAt = time.Now().UTC()

		if err := a.APIKeys.Update(r.Context(), key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update API key"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status": "updated",
			"id":     key.ID.String(),
		})
	}
}

// HandleGetAPIKeySecret returns the plaintext for a key (decrypted from storage).
func HandleGetAPIKeySecret(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		keyID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid key ID"})
			return
		}
		key, err := a.APIKeys.FindByID(r.Context(), keyID)
		if err != nil || key == nil || key.UserID != userID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
			return
		}

		// Legacy keys: no encrypted_key, regenerate once and store it.
		if key.EncryptedKey == "" {
			plaintext, newHash, _, err := generateAPIKeyMaterial(a.Config.Encryption.Key)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate key"})
				return
			}
			encrypted, err := encryptPlaintext(a, plaintext)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to encrypt key"})
				return
			}
			key.KeyHash = newHash
			key.EncryptedKey = encrypted
			key.UpdatedAt = time.Now().UTC()
			if err := a.APIKeys.Update(r.Context(), key); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update key"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"plaintext": plaintext, "regenerated": "true"})
			return
		}

		// Decrypt the stored key.
		plaintext, err := decryptPlaintext(a, key.EncryptedKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to decrypt key"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"plaintext": plaintext})
	}
}

// HandleDeleteAPIKey soft-deletes an API key (sets status to revoked).
func HandleDeleteAPIKey(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		keyID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid key ID"})
			return
		}

		key, err := a.APIKeys.FindByID(r.Context(), keyID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "API key not found"})
			return
		}

		if key.UserID != userID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
			return
		}

		now := time.Now().UTC()
		key.Status = domain.APIKeyStatusRevoked
		key.RevokedAt = &now
		key.UpdatedAt = now

		if err := a.APIKeys.Update(r.Context(), key); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete API key"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":     "deleted",
			"id":         key.ID.String(),
			"revoked_at": now.Format(time.RFC3339),
		})
	}
}

// encryptPlaintext encrypts a plaintext API key using the app's encryption key.
func encryptPlaintext(a *app.App, plaintext string) (string, error) {
	key := []byte(a.Config.Encryption.Key)
	return encrypt.Encrypt(plaintext, key)
}

// decryptPlaintext decrypts an encrypted API key using the app's encryption key.
func decryptPlaintext(a *app.App, hexCiphertext string) (string, error) {
	key := []byte(a.Config.Encryption.Key)
	return encrypt.Decrypt(hexCiphertext, key)
}

// generateAPIKeyMaterial creates random key bytes, returns plaintext, SHA-256 hash, and masked representation.
func generateAPIKeyMaterial(hmacSecret string) (plaintext, keyHash, maskedKey string, err error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", "", fmt.Errorf("generateAPIKeyMaterial: failed to read random bytes: %w", err)
	}

	randomHex := hex.EncodeToString(randomBytes)
	plaintext = apiKeyPrefix + randomHex

	keyHash = keyhash.Hash(plaintext, hmacSecret)

	// Mask: show prefix + first 4 chars of random part, hide the rest.
	maskedKey = fmt.Sprintf("%s%s****%s", apiKeyPrefix, randomHex[:4], randomHex[len(randomHex)-4:])

	return plaintext, keyHash, maskedKey, nil
}

package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/repository/apikey"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

// appForModelsTest creates a minimal App with repos wired for gateway models tests.
func appForModelsTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-gateway-models-32",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Models:  model.NewPostgresRepository(pool),
		APIKeys: apikey.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedModelsForGatewayTest inserts active models into the database.
func seedModelsForGatewayTest(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()

	chatID := uuid.New()
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'deepseek-chat', 'deepseek', 'chat', 'DeepSeek Chat', 'DeepSeek V3 chat model', 128000, 'active', 'GA', NOW(), NOW())`,
		chatID,
	)
	if err != nil {
		t.Fatalf("insert model deepseek-chat: %v", err)
	}

	glmID := uuid.New()
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'glm-4', 'zhipu', 'chat', 'GLM-4', 'Zhipu GLM-4 model', 200000, 'active', 'GA', NOW(), NOW())`,
		glmID,
	)
	if err != nil {
		t.Fatalf("insert model glm-4: %v", err)
	}

	qwenID := uuid.New()
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'qwen-max', 'alibaba', 'chat', 'Qwen Max', 'Qwen max model', 128000, 'beta', 'beta', NOW(), NOW())`,
		qwenID,
	)
	if err != nil {
		t.Fatalf("insert model qwen-max: %v", err)
	}

	// Only channel-backed models are routable and appear in /v1/models.
	for _, mid := range []uuid.UUID{chatID, glmID, qwenID} {
		if _, err := a.Pool.Exec(ctx,
			`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
			 VALUES ($1, $2, $3, 'shared', 'active', NOW(), NOW())`,
			uuid.New(), "gw-channel", mid); err != nil {
			t.Fatalf("insert channel: %v", err)
		}
	}

	// Create an inactive model that should NOT appear in results.
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'deepseek-archive', 'deepseek', 'chat', 'Deprecated', 'deprecated model', 4096, 'inactive', 'unsupported', NOW(), NOW())`,
		uuid.New(),
	)
	if err != nil {
		t.Fatalf("insert model deepseek-archive: %v", err)
	}
}

// seedUserAndKeyForGatewayTest creates a user and an API key, returning the key's UUID.
func seedUserAndKeyForGatewayTest(t *testing.T, a *app.App, allowedModels []string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	userID := uuid.New()
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, userID.String()+"@test.com", "hash",
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	keyID := uuid.New()
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, status, allowed_models, cumulative_limit, weekly_limit, monthly_limit, over_limit_action, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())`,
		keyID, userID, "sk-gw", "hash_gateway_models", "sk-gw...xxx", "test-key", domain.APIKeyStatusActive,
		allowedModels, "0", "0", "0", domain.OverLimitBlock,
	)
	if err != nil {
		t.Fatalf("seed api key: %v", err)
	}

	return keyID
}

// setAPIKeyInContext adds the API key ID to the request context (simulating GatewayAuth).
func setAPIKeyInContext(r *http.Request, keyID string) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.CtxAPIKeyID, keyID)
	return r.WithContext(ctx)
}

// modelResponse represents a single model entry in the OpenAI /v1/models response.
type modelResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// listModelsResponse represents the OpenAI /v1/models list response envelope.
type listModelsResponse struct {
	Object string          `json:"object"`
	Data   []modelResponse `json:"data"`
}

func TestHandleListModels_ReturnsActiveModelsFromDB(t *testing.T) {
	// Arrange
	a := appForModelsTest(t)
	seedModelsForGatewayTest(t, a)
	keyID := seedUserAndKeyForGatewayTest(t, a, nil)

	handler := HandleListModels(a)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = setAPIKeyInContext(req, keyID.String())
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp listModelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("object = %s, want list", resp.Object)
	}

	if len(resp.Data) < 3 {
		t.Errorf("expected at least 3 active models, got %d", len(resp.Data))
	}

	// Verify each model has the correct format.
	modelCodes := make(map[string]bool)
	for _, m := range resp.Data {
		if m.Object != "model" {
			t.Errorf("model %s: object = %s, want model", m.ID, m.Object)
		}
		if m.ID == "" {
			t.Error("model has empty ID")
		}
		if m.OwnedBy == "" {
			t.Errorf("model %s: owned_by is empty", m.ID)
		}
		if m.Created <= 0 {
			t.Errorf("model %s: created = %d, want positive unix timestamp", m.ID, m.Created)
		}
		modelCodes[m.ID] = true
	}

	// Active models should be present.
	if !modelCodes["deepseek-chat"] {
		t.Error("expected deepseek-chat in active models")
	}
	if !modelCodes["glm-4"] {
		t.Error("expected glm-4 in active models")
	}
	if !modelCodes["qwen-max"] {
		t.Error("expected qwen-max (beta) in active models")
	}
	// Inactive model should NOT be present.
	if modelCodes["deepseek-archive"] {
		t.Error("deepseek-archive should not be in active models list")
	}
}

func TestHandleListModels_KeyWithAllowedModels_FiltersCorrectly(t *testing.T) {
	// Arrange
	a := appForModelsTest(t)
	seedModelsForGatewayTest(t, a)
	keyID := seedUserAndKeyForGatewayTest(t, a, []string{"deepseek-chat", "qwen-max"})

	handler := HandleListModels(a)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = setAPIKeyInContext(req, keyID.String())
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp listModelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("object = %s, want list", resp.Object)
	}

	if len(resp.Data) != 2 {
		t.Errorf("expected exactly 2 allowed models, got %d: %v", len(resp.Data), resp.Data)
	}

	modelCodes := make(map[string]bool)
	for _, m := range resp.Data {
		modelCodes[m.ID] = true
	}

	if !modelCodes["deepseek-chat"] {
		t.Error("expected deepseek-chat in filtered results")
	}
	if !modelCodes["qwen-max"] {
		t.Error("expected qwen-max in filtered results")
	}
	if modelCodes["glm-4"] {
		t.Error("glm-4 should NOT be in filtered results (not in AllowedModels)")
	}
}

func TestHandleListModels_NoModelsInDB_ReturnsEmptyList(t *testing.T) {
	// Arrange
	a := appForModelsTest(t)
	// Do NOT seed any models - truncate all was called in appForModelsTest.
	keyID := seedUserAndKeyForGatewayTest(t, a, nil)

	handler := HandleListModels(a)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req = setAPIKeyInContext(req, keyID.String())
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp listModelsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("object = %s, want list", resp.Object)
	}

	if len(resp.Data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(resp.Data))
	}
}

func TestHandleListModels_NoKeyInContext_Returns401(t *testing.T) {
	// Arrange
	a := appForModelsTest(t)
	seedModelsForGatewayTest(t, a)

	handler := HandleListModels(a)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	// Intentionally do NOT set API key in context.
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}

	var errResp struct {
		Error map[string]string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error["type"] == "" {
		t.Error("expected error type in response")
	}
}

func TestHandleListModels_InvalidKeyIDInContext_Returns401(t *testing.T) {
	// Arrange
	a := appForModelsTest(t)
	seedModelsForGatewayTest(t, a)

	handler := HandleListModels(a)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	// Set an invalid UUID as the API key ID.
	req = setAPIKeyInContext(req, "not-a-valid-uuid")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestHandleListModels_NonexistentKeyID_Returns401(t *testing.T) {
	// Arrange
	a := appForModelsTest(t)
	seedModelsForGatewayTest(t, a)

	handler := HandleListModels(a)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	// Set a valid-format UUID that does not exist in the DB.
	req = setAPIKeyInContext(req, uuid.New().String())
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestHandleListModels_OnlyGetMethodAllowed(t *testing.T) {
	// Arrange
	a := appForModelsTest(t)

	handler := HandleListModels(a)
	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	req = setAPIKeyInContext(req, uuid.New().String())
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

package console

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// appForModelsTest creates a minimal App with repos wired for model tests.
func appForModelsTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-models-32byte",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Users:   user.NewPostgresRepository(pool),
		Models:  model.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// setUserInModelsContext adds a user ID to the request context.
func setUserInModelsContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	return r.WithContext(ctx)
}

// seedModelsForTest inserts active models in the database.
func seedModelsForTest(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()

	// Insert models directly via SQL for complete control over pricing
	model1ID := uuid.New()
	model2ID := uuid.New()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'gpt-4o', 'openai', 'chat', 'GPT-4o', 'OpenAI GPT-4o model', 128000, 'active', 'GA', NOW(), NOW())`,
		model1ID,
	)
	if err != nil {
		t.Fatalf("insert model1: %v", err)
	}

	_, err = a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'claude-sonnet', 'anthropic', 'chat', 'Claude Sonnet 4', 'Anthropic Claude model', 200000, 'active', 'GA', NOW(), NOW())`,
		model2ID,
	)
	if err != nil {
		t.Fatalf("insert model2: %v", err)
	}

	// Insert pricing for model 1
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, created_at, updated_at)
		 VALUES ($1, $2, 'chat', 'input', 'token', '2.50', 'CNY', '1.20', TRUE, NOW(), NOW())`,
		uuid.New(), model1ID,
	)
	if err != nil {
		t.Fatalf("insert pricing model1 input: %v", err)
	}
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, created_at, updated_at)
		 VALUES ($1, $2, 'chat', 'output', 'token', '10.00', 'CNY', '5.00', TRUE, NOW(), NOW())`,
		uuid.New(), model1ID,
	)
	if err != nil {
		t.Fatalf("insert pricing model1 output: %v", err)
	}

	// Insert pricing for model 2
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, created_at, updated_at)
		 VALUES ($1, $2, 'chat', 'input', 'token', '3.00', 'CNY', '1.50', TRUE, NOW(), NOW())`,
		uuid.New(), model2ID,
	)
	if err != nil {
		t.Fatalf("insert pricing model2 input: %v", err)
	}
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, created_at, updated_at)
		 VALUES ($1, $2, 'chat', 'output', 'token', '15.00', 'CNY', '7.50', TRUE, NOW(), NOW())`,
		uuid.New(), model2ID,
	)
	if err != nil {
		t.Fatalf("insert pricing model2 output: %v", err)
	}
}

// seedDomesticModelsForListTest seeds two active domestic-provider models so
// the console list handler's domestic-provider filter (IsDomesticProvider)
// keeps them visible. Pricing mirrors seedModelsForTest for the list test.
func seedDomesticModelsForListTest(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()

	model1ID := uuid.New()
	model2ID := uuid.New()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'deepseek-chat', 'deepseek', 'chat', 'DeepSeek Chat', 'DeepSeek V3 chat model', 128000, 'active', 'GA', NOW(), NOW())`,
		model1ID,
	)
	if err != nil {
		t.Fatalf("insert model1: %v", err)
	}

	_, err = a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, description, context_window, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'glm-4', 'zhipu', 'chat', 'GLM-4', 'Zhipu GLM-4 model', 200000, 'active', 'GA', NOW(), NOW())`,
		model2ID,
	)
	if err != nil {
		t.Fatalf("insert model2: %v", err)
	}

	// Only channel-backed models are routable and must appear in catalogs.
	for _, mid := range []uuid.UUID{model1ID, model2ID} {
		if _, err := a.Pool.Exec(context.Background(),
			`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
			 VALUES ($1, $2, $3, 'shared', 'active', NOW(), NOW())`,
			uuid.New(), "seed-channel", mid); err != nil {
			t.Fatalf("seed channel: %v", err)
		}
	}

	for _, row := range []struct {
		modelID uuid.UUID
		input   string
		output  string
		inCost  string
		outCost string
	}{
		{model1ID, "2.50", "10.00", "1.20", "5.00"},
		{model2ID, "3.00", "15.00", "1.50", "7.50"},
	} {
		if _, err := a.Pool.Exec(ctx,
			`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, created_at, updated_at)
			 VALUES ($1, $2, 'chat', 'input', 'token', $3, 'CNY', $4, TRUE, NOW(), NOW())`,
			uuid.New(), row.modelID, row.input, row.inCost); err != nil {
			t.Fatalf("insert pricing input: %v", err)
		}
		if _, err := a.Pool.Exec(ctx,
			`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, created_at, updated_at)
			 VALUES ($1, $2, 'chat', 'output', 'token', $3, 'CNY', $4, TRUE, NOW(), NOW())`,
			uuid.New(), row.modelID, row.output, row.outCost); err != nil {
			t.Fatalf("insert pricing output: %v", err)
		}
	}
}

// =============================================================================
// HandleListModels Tests
// =============================================================================

func TestHandleListModels_ReturnsActiveModels(t *testing.T) {
	a := appForModelsTest(t)
	seedDomesticModelsForListTest(t, a)

	req := httptest.NewRequest(http.MethodGet, "/api/console/models", nil)
	w := httptest.NewRecorder()

	handler := HandleListModels(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			Code          string         `json:"code"`
			Provider      string         `json:"provider"`
			Category      string         `json:"category"`
			DisplayName   string         `json:"display_name"`
			ContextWindow int            `json:"context_window"`
			Pricing       map[string]any `json:"pricing"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	// Verify model data
	foundGPT, foundClaude := false, false
	for _, m := range resp.Data {
		switch m.Code {
		case "deepseek-chat":
			foundGPT = true
			if m.Provider != "deepseek" {
				t.Errorf("deepseek-chat provider = %s, want 'deepseek'", m.Provider)
			}
			if m.Category != "chat" {
				t.Errorf("deepseek-chat category = %s, want 'chat'", m.Category)
			}
			if m.ContextWindow != 128000 {
				t.Errorf("deepseek-chat context_window = %d, want 128000", m.ContextWindow)
			}
			if m.Pricing == nil {
				t.Error("deepseek-chat pricing should not be nil")
			}
			if m.Pricing["input"] != "2.50" {
				t.Errorf("deepseek-chat input price = %v, want '2.50'", m.Pricing["input"])
			}
			if m.Pricing["output"] != "10.00" {
				t.Errorf("deepseek-chat output price = %v, want '10.00'", m.Pricing["output"])
			}
		case "glm-4":
			foundClaude = true
			if m.Provider != "zhipu" {
				t.Errorf("glm-4 provider = %s, want 'zhipu'", m.Provider)
			}
			if m.ContextWindow != 200000 {
				t.Errorf("glm-4 context_window = %d, want 200000", m.ContextWindow)
			}
			if m.Pricing == nil {
				t.Error("glm-4 pricing should not be nil")
			}
		}
	}
	if !foundGPT {
		t.Error("deepseek-chat not found in response")
	}
	if !foundClaude {
		t.Error("glm-4 not found in response")
	}
}

func TestHandleListModels_OnlyActiveModels(t *testing.T) {
	a := appForModelsTest(t)
	ctx := context.Background()

	// Insert one active and one inactive model
	activeID := uuid.New()
	inactiveID := uuid.New()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'active-model', 'deepseek', 'chat', 'Active', 'active', 'GA', NOW(), NOW())`,
		activeID,
	)
	if err != nil {
		t.Fatalf("insert active model: %v", err)
	}
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'inactive-model', 'deepseek', 'chat', 'Inactive', 'inactive', 'unsupported', NOW(), NOW())`,
		inactiveID,
	)
	if err != nil {
		t.Fatalf("insert inactive model: %v", err)
	}
	// The active model needs a channel to be routable and listable.
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', NOW(), NOW())`,
		uuid.New(), "active-model-channel", activeID)
	if err != nil {
		t.Fatalf("insert active channel: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/models", nil)
	w := httptest.NewRecorder()

	handler := HandleListModels(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			Code string `json:"code"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 active model, got %d", len(resp.Data))
	}
	if resp.Data[0].Code != "active-model" {
		t.Errorf("code = %s, want 'active-model'", resp.Data[0].Code)
	}
}

func TestHandleListModels_EmptyList(t *testing.T) {
	a := appForModelsTest(t)
	// No models seeded

	req := httptest.NewRequest(http.MethodGet, "/api/console/models", nil)
	w := httptest.NewRecorder()

	handler := HandleListModels(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty array, got %d items", len(resp.Data))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}

func TestHandleListModels_ResponseContentTypeJSON(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/models", nil)
	w := httptest.NewRecorder()

	handler := HandleListModels(a)
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

// =============================================================================
// HandleLoginHistory Tests
// =============================================================================

// seedUserForLoginHistory creates a user with bcrypt hash.
func seedUserForLoginHistory(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	now := time.Now().UTC()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		Role:         "user",
		UserType:     domain.UserTypePersonal,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("seedUserForLoginHistory: create: %v", err)
	}
	return u
}

// setUserInLoginHistoryContext adds a user ID to the request context.
func setUserInLoginHistoryContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	return r.WithContext(ctx)
}

func TestHandleLoginHistory_NoAuth(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/security/login-history", nil)
	w := httptest.NewRecorder()

	handler := HandleLoginHistory(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleLoginHistory_EmptyHistory(t *testing.T) {
	a := appForModelsTest(t)
	seedUser := seedUserForLoginHistory(t, a, "empty-history@example.com", "pass", "Empty History")

	req := httptest.NewRequest(http.MethodGet, "/api/console/security/login-history", nil)
	req = setUserInLoginHistoryContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleLoginHistory(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty array, got %d items", len(resp.Data))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}

func TestHandleLoginHistory_WithHistory(t *testing.T) {
	a := appForModelsTest(t)
	seedUser := seedUserForLoginHistory(t, a, "has-history@example.com", "pass", "Has History")

	// Insert login history records
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO login_history (id, user_id, ip_address, user_agent, success, created_at)
		 VALUES ($1, $2, '192.168.1.100', 'Chrome/126.0', true, $3)`,
		uuid.New(), seedUser.ID, now,
	)
	if err != nil {
		t.Fatalf("insert login history 1: %v", err)
	}
	_, err = a.Pool.Exec(context.Background(),
		`INSERT INTO login_history (id, user_id, ip_address, user_agent, success, created_at)
		 VALUES ($1, $2, '10.0.0.50', 'Firefox/125.0', false, $3)`,
		uuid.New(), seedUser.ID, now.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("insert login history 2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/security/login-history", nil)
	req = setUserInLoginHistoryContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleLoginHistory(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			IPAddress string `json:"ip_address"`
			UserAgent string `json:"user_agent"`
			Success   bool   `json:"success"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	// Most recent first
	if resp.Data[0].IPAddress != "192.168.1.100" {
		t.Errorf("first entry ip_address = %s, want '192.168.1.100'", resp.Data[0].IPAddress)
	}
	if resp.Data[0].UserAgent != "Chrome/126.0" {
		t.Errorf("first entry user_agent = %s, want 'Chrome/126.0'", resp.Data[0].UserAgent)
	}
	if !resp.Data[0].Success {
		t.Error("first entry success should be true")
	}
}

func TestHandleLoginHistory_OnlyOwnHistory(t *testing.T) {
	a := appForModelsTest(t)
	ctx := context.Background()
	userA := seedUserForLoginHistory(t, a, "lh-userA@example.com", "passA", "LH User A")
	userB := seedUserForLoginHistory(t, a, "lh-userB@example.com", "passB", "LH User B")

	// Insert history for both users
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO login_history (id, user_id, ip_address, user_agent, success, created_at)
		 VALUES ($1, $2, '1.1.1.1', 'Agent-A', true, NOW())`,
		uuid.New(), userA.ID,
	)
	if err != nil {
		t.Fatalf("insert history A: %v", err)
	}
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO login_history (id, user_id, ip_address, user_agent, success, created_at)
		 VALUES ($1, $2, '2.2.2.2', 'Agent-B', true, NOW())`,
		uuid.New(), userB.ID,
	)
	if err != nil {
		t.Fatalf("insert history B: %v", err)
	}

	// Request as user A
	req := httptest.NewRequest(http.MethodGet, "/api/console/security/login-history", nil)
	req = setUserInLoginHistoryContext(req, userA.ID.String())
	w := httptest.NewRecorder()

	handler := HandleLoginHistory(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			IPAddress string `json:"ip_address"`
			UserAgent string `json:"user_agent"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 entry for user A, got %d", len(resp.Data))
	}
	if resp.Data[0].IPAddress != "1.1.1.1" {
		t.Errorf("ip_address = %s, want '1.1.1.1'", resp.Data[0].IPAddress)
	}
}

func TestHandleLoginHistory_Max50Entries(t *testing.T) {
	a := appForModelsTest(t)
	ctx := context.Background()
	seedUser := seedUserForLoginHistory(t, a, "lh-limit@example.com", "pass", "LH Limit")

	// Insert 55 login history entries
	for i := 0; i < 55; i++ {
		secs := fmt.Sprintf("%d", i)
		_, err := a.Pool.Exec(ctx,
			`INSERT INTO login_history (id, user_id, ip_address, user_agent, success, created_at)
			 VALUES ($1, $2, '10.0.0.1', 'TestAgent', true, NOW() - ($3 || ' seconds')::INTERVAL)`,
			uuid.New(), seedUser.ID, secs,
		)
		if err != nil {
			t.Fatalf("insert history %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/security/login-history", nil)
	req = setUserInLoginHistoryContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleLoginHistory(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) > 50 {
		t.Errorf("expected at most 50 entries, got %d", len(resp.Data))
	}
	if len(resp.Data) < 50 {
		t.Errorf("expected exactly 50 entries (LIMIT 50), got %d", len(resp.Data))
	}
}

// =============================================================================
// CRUD Tests: HandleCreateModel / HandleUpdateModel / HandleDeleteModel / HandleGetModel
// =============================================================================

type testPricingRequest struct {
	Dimension  string         `json:"dimension"`
	UnitName   string         `json:"unit_name"`
	UnitPrice  string         `json:"unit_price"`
	Conditions map[string]any `json:"conditions,omitempty"`
}

type testCreateModelRequest struct {
	Code          string               `json:"code"`
	Provider      string               `json:"provider"`
	Category      string               `json:"category"`
	DisplayName   string               `json:"display_name,omitempty"`
	ContextWindow int                  `json:"context_window,omitempty"`
	Pricings      []testPricingRequest `json:"pricings,omitempty"`
}

func TestHandleCreateModel_NoAuth(t *testing.T) {
	a := appForModelsTest(t)

	body := testCreateModelRequest{
		Code:     "test-model",
		Provider: "openai",
		Category: "chat",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/models", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleCreateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateModel_MissingRequiredFields(t *testing.T) {
	a := appForModelsTest(t)

	tests := []struct {
		name string
		body testCreateModelRequest
	}{
		{"missing code", testCreateModelRequest{Provider: "openai", Category: "chat"}},
		{"missing provider", testCreateModelRequest{Code: "test", Category: "chat"}},
		{"missing category", testCreateModelRequest{Code: "test", Provider: "openai"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/console/models", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setUserInModelsContext(req, uuid.New().String())
			w := httptest.NewRecorder()

			handler := HandleCreateModel(a)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

func TestHandleCreateModel_Success(t *testing.T) {
	a := appForModelsTest(t)

	body := testCreateModelRequest{
		Code:          "gpt-4o-mini",
		Provider:      "openai",
		Category:      "chat",
		DisplayName:   "GPT-4o Mini",
		ContextWindow: 128000,
		Pricings: []testPricingRequest{
			{Dimension: "input", UnitName: "token", UnitPrice: "0.15"},
			{Dimension: "output", UnitName: "token", UnitPrice: "0.60"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/models", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler := HandleCreateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID            string `json:"id"`
			Code          string `json:"code"`
			Provider      string `json:"provider"`
			Category      string `json:"category"`
			DisplayName   string `json:"display_name"`
			ContextWindow int    `json:"context_window"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.ID == "" {
		t.Fatal("id should not be empty")
	}
	if resp.Data.Code != body.Code {
		t.Errorf("code = %s, want %s", resp.Data.Code, body.Code)
	}
	if resp.Data.Provider != body.Provider {
		t.Errorf("provider = %s, want %s", resp.Data.Provider, body.Provider)
	}
	if resp.Data.Category != body.Category {
		t.Errorf("category = %s, want %s", resp.Data.Category, body.Category)
	}
	if resp.Data.DisplayName != body.DisplayName {
		t.Errorf("display_name = %s, want %s", resp.Data.DisplayName, body.DisplayName)
	}
	if resp.Data.ContextWindow != body.ContextWindow {
		t.Errorf("context_window = %d, want %d", resp.Data.ContextWindow, body.ContextWindow)
	}

	// Verify pricing was created
	modelID, err := uuid.Parse(resp.Data.ID)
	if err != nil {
		t.Fatalf("parse model ID: %v", err)
	}

	rows, err := a.Pool.Query(context.Background(),
		`SELECT pricing_dimension, unit_price FROM model_pricing WHERE model_id = $1 ORDER BY pricing_dimension`, modelID)
	if err != nil {
		t.Fatalf("query pricing: %v", err)
	}
	defer rows.Close()

	pricingCount := 0
	for rows.Next() {
		var dim, price string
		if err := rows.Scan(&dim, &price); err != nil {
			t.Fatalf("scan pricing: %v", err)
		}
		pricingCount++
		for _, p := range body.Pricings {
			if p.Dimension == dim && trimDecimalPrice(p.UnitPrice) != trimDecimalPrice(price) {
				t.Errorf("pricing %s price = %s, want %s", dim, trimDecimalPrice(price), trimDecimalPrice(p.UnitPrice))
			}
		}
	}
	if pricingCount != 2 {
		t.Errorf("expected 2 pricing rows, got %d", pricingCount)
	}
}

func TestHandleCreateModel_DuplicateCode(t *testing.T) {
	a := appForModelsTest(t)
	seedModelsForTest(t, a) // seeds "gpt-4o" and "claude-sonnet"

	body := testCreateModelRequest{
		Code:     "gpt-4o",
		Provider: "openai",
		Category: "chat",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/models", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler := HandleCreateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestHandleCreateModel_InvalidBody(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/console/models", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler := HandleCreateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// HandleUpdateModel Tests
// =============================================================================

func TestHandleUpdateModel_NoAuth(t *testing.T) {
	a := appForModelsTest(t)

	body := map[string]string{"display_name": "Updated"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/console/models/"+uuid.New().String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleUpdateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUpdateModel_NotFound(t *testing.T) {
	a := appForModelsTest(t)

	body := map[string]string{"display_name": "Updated"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/console/models/"+uuid.New().String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", uuid.New().String())
	w := httptest.NewRecorder()

	handler := HandleUpdateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleUpdateModel_Success(t *testing.T) {
	a := appForModelsTest(t)
	seedModelsForTest(t, a)

	found, err := a.Models.FindByCode(context.Background(), "gpt-4o")
	if err != nil || found == nil {
		t.Fatalf("FindByCode gpt-4o: %v", err)
	}
	modelID := found.ID

	body := map[string]interface{}{
		"display_name":   "GPT-4o Updated",
		"context_window": 256000,
		"status":         "active",
		"pricings": []map[string]string{
			{"dimension": "input", "unit_name": "token", "unit_price": "0.50"},
			{"dimension": "output", "unit_name": "token", "unit_price": "2.00"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/console/models/"+modelID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", modelID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID            string `json:"id"`
			Code          string `json:"code"`
			DisplayName   string `json:"display_name"`
			ContextWindow int    `json:"context_window"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.DisplayName != "GPT-4o Updated" {
		t.Errorf("display_name = %s, want 'GPT-4o Updated'", resp.Data.DisplayName)
	}
	if resp.Data.ContextWindow != 256000 {
		t.Errorf("context_window = %d, want 256000", resp.Data.ContextWindow)
	}

	// Verify pricing was updated
	rows, err := a.Pool.Query(context.Background(),
		`SELECT pricing_dimension, unit_price FROM model_pricing WHERE model_id = $1 ORDER BY pricing_dimension`, modelID)
	if err != nil {
		t.Fatalf("query pricing: %v", err)
	}
	defer rows.Close()

	pricingMap := make(map[string]string)
	for rows.Next() {
		var dim, price string
		if err := rows.Scan(&dim, &price); err != nil {
			t.Fatalf("scan pricing: %v", err)
		}
		pricingMap[dim] = price
	}
	if trimDecimalPrice(pricingMap["input"]) != "0.50" {
		t.Errorf("input price = %s, want 0.50", trimDecimalPrice(pricingMap["input"]))
	}
	if trimDecimalPrice(pricingMap["output"]) != "2.00" {
		t.Errorf("output price = %s, want 2.00", trimDecimalPrice(pricingMap["output"]))
	}
}

func TestHandleUpdateModel_InvalidID(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/console/models/not-a-uuid", nil)
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	handler := HandleUpdateModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// HandleDeleteModel Tests
// =============================================================================

func TestHandleDeleteModel_NoAuth(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/models/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()

	handler := HandleDeleteModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteModel_NotFound(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/models/"+uuid.New().String(), nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", uuid.New().String())
	w := httptest.NewRecorder()

	handler := HandleDeleteModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleDeleteModel_Success(t *testing.T) {
	a := appForModelsTest(t)
	seedModelsForTest(t, a)

	found, err := a.Models.FindByCode(context.Background(), "claude-sonnet")
	if err != nil || found == nil {
		t.Fatalf("FindByCode claude-sonnet: %v", err)
	}
	modelID := found.ID

	req := httptest.NewRequest(http.MethodDelete, "/api/console/models/"+modelID.String(), nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", modelID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "deleted" {
		t.Errorf("status = %s, want 'deleted'", resp["status"])
	}

	// Verify model is soft-deleted in DB
	var dbStatus string
	err = a.Pool.QueryRow(context.Background(),
		`SELECT status FROM models WHERE id = $1`, modelID).Scan(&dbStatus)
	if err != nil {
		t.Fatalf("query model status: %v", err)
	}
	if dbStatus != "inactive" {
		t.Errorf("db status = %s, want 'inactive'", dbStatus)
	}
}

func TestHandleDeleteModel_InvalidID(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/models/not-a-uuid", nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	handler := HandleDeleteModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// HandleGetModel Tests
// =============================================================================

func TestHandleGetModel_NotFound(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/models/"+uuid.New().String(), nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", uuid.New().String())
	w := httptest.NewRecorder()

	handler := HandleGetModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleGetModel_Success(t *testing.T) {
	a := appForModelsTest(t)
	seedModelsForTest(t, a)

	found, err := a.Models.FindByCode(context.Background(), "gpt-4o")
	if err != nil || found == nil {
		t.Fatalf("FindByCode gpt-4o: %v", err)
	}
	modelID := found.ID

	req := httptest.NewRequest(http.MethodGet, "/api/console/models/"+modelID.String(), nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", modelID.String())
	w := httptest.NewRecorder()

	handler := HandleGetModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID            string         `json:"id"`
			Code          string         `json:"code"`
			Provider      string         `json:"provider"`
			Category      string         `json:"category"`
			DisplayName   string         `json:"display_name"`
			ContextWindow int            `json:"context_window"`
			Status        string         `json:"status"`
			Pricing       map[string]any `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.ID != modelID.String() {
		t.Errorf("id = %s, want %s", resp.Data.ID, modelID.String())
	}
	if resp.Data.Code != "gpt-4o" {
		t.Errorf("code = %s, want 'gpt-4o'", resp.Data.Code)
	}
	if resp.Data.Provider != "openai" {
		t.Errorf("provider = %s, want 'openai'", resp.Data.Provider)
	}
	if resp.Data.DisplayName != "GPT-4o" {
		t.Errorf("display_name = %s, want 'GPT-4o'", resp.Data.DisplayName)
	}
	if resp.Data.ContextWindow != 128000 {
		t.Errorf("context_window = %d, want 128000", resp.Data.ContextWindow)
	}
	if resp.Data.Pricing == nil {
		t.Error("pricing should not be nil")
	}
	if resp.Data.Pricing["input"] != "2.50" {
		t.Errorf("input price = %v, want '2.50'", resp.Data.Pricing["input"])
	}
	if resp.Data.Pricing["output"] != "10.00" {
		t.Errorf("output price = %v, want '10.00'", resp.Data.Pricing["output"])
	}
}

func TestHandleGetModel_InvalidID(t *testing.T) {
	a := appForModelsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/models/not-a-uuid", nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	handler := HandleGetModel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ============================================================================
// RED: a cost row smuggled into a model edit must never be re-inserted as a
// sell row. Cost rows are owned by provider cost sync; editing a model's sell
// prices must leave every cost row untouched.
// ============================================================================

func TestHandleUpdateModel_CostRowInRequest_Ignored(t *testing.T) {
	a := appForModelsTest(t)
	seedModelsForTest(t, a)

	found, err := a.Models.FindByCode(context.Background(), "gpt-4o")
	if err != nil || found == nil {
		t.Fatalf("FindByCode gpt-4o: %v", err)
	}
	modelID := found.ID

	// Insert an explicit cost row for input (price_type='cost').
	_, err = a.Pool.Exec(context.Background(),
		`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, is_active, period, price_type, created_at, updated_at)
		 VALUES ($1, $2, 'chat', 'input', '1K tokens', '0.0010', 'CNY', TRUE, 'off_peak', 'cost', NOW(), NOW())`,
		uuid.New(), modelID)
	if err != nil {
		t.Fatalf("insert cost row: %v", err)
	}

	// The admin UI only edits sell rows, but a stale/buggy client could send
	// the read-only cost row back. The update must ignore it.
	body := map[string]interface{}{
		"pricings": []map[string]string{
			{"dimension": "input", "unit_name": "1K tokens", "unit_price": "0.0020", "price_type": "cost"},
			{"dimension": "output", "unit_name": "1K tokens", "unit_price": "1.00", "price_type": "sell"},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/models/"+modelID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", modelID.String())
	w := httptest.NewRecorder()
	HandleUpdateModel(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	rows, err := a.Pool.Query(context.Background(),
		`SELECT pricing_dimension, price_type, unit_price FROM model_pricing WHERE model_id = $1 AND is_active = TRUE ORDER BY pricing_dimension, price_type`,
		modelID)
	if err != nil {
		t.Fatalf("query pricing: %v", err)
	}
	defer rows.Close()

	type row struct {
		dim, priceType, price string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.dim, &r.priceType, &r.price); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	for _, r := range got {
		if r.dim == "input" && r.priceType == "sell" {
			t.Fatalf("input sell row appeared after edit: %+v — cost rows must not be re-inserted as sell", r)
		}
		if r.dim == "input" && r.priceType == "cost" && trimDecimalPrice(r.price) != "0.001" {
			t.Errorf("input cost price = %s, want 0.001 (cost row must survive untouched)", trimDecimalPrice(r.price))
		}
	}
	foundSellOutput := false
	for _, r := range got {
		if r.dim == "output" && r.priceType == "sell" && trimDecimalPrice(r.price) == "1.00" {
			foundSellOutput = true
		}
	}
	if !foundSellOutput {
		t.Error("output sell row with price 1.00 was not stored")
	}
}

// ============================================================================
// RED: a model price edit must leave an audit trail with the pre-edit price
// snapshot so price changes are attributable.
// ============================================================================

func TestHandleUpdateModel_AuditRecordsOldPricing(t *testing.T) {
	a := appForModelsTest(t)
	seedModelsForTest(t, a)

	found, err := a.Models.FindByCode(context.Background(), "gpt-4o")
	if err != nil || found == nil {
		t.Fatalf("FindByCode gpt-4o: %v", err)
	}
	modelID := found.ID

	// The seeded gpt-4o input sell price is 2.50; the audit snapshot must
	// capture that pre-edit value.
	actorID := uuid.New()
	_, err = a.Pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, status, created_at, updated_at)
		 VALUES ($1, 'audit-model@test.local', 'x', 'active', NOW(), NOW())`, actorID)
	if err != nil {
		t.Fatalf("insert audit actor: %v", err)
	}

	body := map[string]interface{}{
		"pricings": []map[string]string{
			{"dimension": "input", "unit_name": "1K tokens", "unit_price": "2.00", "price_type": "sell"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	mw := middleware.AuditAdminWrite(a.Pool)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleUpdateModel(a).ServeHTTP(w, r)
	}))

	req := httptest.NewRequest(http.MethodPut, "/api/admin/models/"+modelID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.11:4321"
	req = setUserInModelsContext(req, actorID.String())
	req = chiRouteCtx(req, "id", modelID.String())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var oldValue []byte
	err = a.Pool.QueryRow(context.Background(),
		`SELECT old_value FROM audit_logs WHERE resource_type = 'models' AND action LIKE 'PUT %' ORDER BY created_at DESC LIMIT 1`,
	).Scan(&oldValue)
	if err != nil {
		t.Fatalf("query audit old_value: %v", err)
	}
	if !bytes.Contains(oldValue, []byte("2.50")) {
		t.Errorf("audit old_value = %s, want it to contain pre-edit price 2.50", oldValue)
	}
}

// ============================================================================
// Tiered pricing: model_pricing.conditions must survive create/update/list/get
// roundtrips so admins can edit tiers from the console (new-api parity), and
// malformed conditions must be rejected instead of silently disabling a tier.
// ============================================================================

func TestHandleCreateModel_TieredConditionsRoundtrip(t *testing.T) {
	a := appForModelsTest(t)

	body := testCreateModelRequest{
		Code:     "deepseek-tiers",
		Provider: "deepseek",
		Category: "chat",
		Pricings: []testPricingRequest{
			{Dimension: "input", UnitName: "1M tokens", UnitPrice: "3.00", Conditions: map[string]any{"max_total_tokens": 200000}},
			{Dimension: "input", UnitName: "1M tokens", UnitPrice: "2.00", Conditions: map[string]any{"min_total_tokens": 200001}},
			{Dimension: "output", UnitName: "1M tokens", UnitPrice: "8.00"},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/console/models", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	w := httptest.NewRecorder()
	HandleCreateModel(a).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}

	// DB rows keep the tier conditions.
	rows, err := a.Pool.Query(context.Background(),
		`SELECT pricing_dimension, unit_price, COALESCE(conditions, '{}'::jsonb)::text
		 FROM model_pricing WHERE model_id = $1 AND price_type = 'sell'
		 ORDER BY pricing_dimension, unit_price`, resp.Data.ID)
	if err != nil {
		t.Fatalf("query pricing: %v", err)
	}
	defer rows.Close()

	type dbRow struct {
		dim, price, cond string
	}
	var got []dbRow
	for rows.Next() {
		var r dbRow
		if err := rows.Scan(&r.dim, &r.price, &r.cond); err != nil {
			t.Fatalf("scan pricing: %v", err)
		}
		got = append(got, r)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 sell rows, got %d: %+v", len(got), got)
	}
	if got[0].dim != "input" || trimDecimalPrice(got[0].price) != "2.00" || !strings.Contains(got[0].cond, "200001") {
		t.Errorf("row 0 = %+v, want input 2.00 with min_total_tokens 200001", got[0])
	}
	if got[1].dim != "input" || trimDecimalPrice(got[1].price) != "3.00" || !strings.Contains(got[1].cond, "200000") {
		t.Errorf("row 1 = %+v, want input 3.00 with max_total_tokens 200000", got[1])
	}
	if got[2].dim != "output" || trimDecimalPrice(got[2].price) != "8.00" || got[2].cond != "{}" {
		t.Errorf("row 2 = %+v, want output 8.00 with empty conditions", got[2])
	}

	// GET roundtrip exposes the conditions to the console UI.
	req = httptest.NewRequest(http.MethodGet, "/api/console/models/"+resp.Data.ID, nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", resp.Data.ID)
	w = httptest.NewRecorder()
	HandleGetModel(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var getResp struct {
		Data struct {
			Pricings []struct {
				Dimension  string         `json:"dimension"`
				UnitPrice  string         `json:"unit_price"`
				Conditions map[string]any `json:"conditions"`
			} `json:"pricings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	inputTiers := 0
	for _, p := range getResp.Data.Pricings {
		if p.Dimension != "input" {
			continue
		}
		inputTiers++
		if trimDecimalPrice(p.UnitPrice) == "2.00" {
			if p.Conditions["min_total_tokens"] != float64(200001) {
				t.Errorf("input 2.00 conditions = %v, want min_total_tokens 200001", p.Conditions)
			}
		}
		if trimDecimalPrice(p.UnitPrice) == "3.00" {
			if p.Conditions["max_total_tokens"] != float64(200000) {
				t.Errorf("input 3.00 conditions = %v, want max_total_tokens 200000", p.Conditions)
			}
		}
	}
	if inputTiers != 2 {
		t.Errorf("GET pricings exposed %d input tiers, want 2", inputTiers)
	}
}

func TestHandleCreateModel_RejectsInvalidTierConditions(t *testing.T) {
	cases := []struct {
		name       string
		conditions map[string]any
	}{
		{"min greater than max", map[string]any{"min_total_tokens": 500, "max_total_tokens": 100}},
		{"unsupported key", map[string]any{"below_tokens": 100}},
		{"non-integer max", map[string]any{"max_total_tokens": "abc"}},
		{"negative min", map[string]any{"min_total_tokens": -1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := appForModelsTest(t)
			body := testCreateModelRequest{
				Code:     "bad-tiers",
				Provider: "deepseek",
				Category: "chat",
				Pricings: []testPricingRequest{
					{Dimension: "input", UnitName: "1M tokens", UnitPrice: "1.00", Conditions: tc.conditions},
				},
			}
			bodyBytes, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/api/console/models", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setUserInModelsContext(req, uuid.New().String())
			w := httptest.NewRecorder()
			HandleCreateModel(a).ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
			}
			var resp map[string]string
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			if resp["error"] == "" {
				t.Error("expected a descriptive error message")
			}
		})
	}
}

func TestHandleUpdateModel_TieredConditionsRoundtrip(t *testing.T) {
	a := appForModelsTest(t)
	seedModelsForTest(t, a)

	found, err := a.Models.FindByCode(context.Background(), "gpt-4o")
	if err != nil || found == nil {
		t.Fatalf("FindByCode gpt-4o: %v", err)
	}
	modelID := found.ID

	body := map[string]interface{}{
		"pricings": []map[string]interface{}{
			{"dimension": "input", "unit_name": "1M tokens", "unit_price": "2.00", "conditions": map[string]any{"max_total_tokens": 200000}},
			{"dimension": "output", "unit_name": "1M tokens", "unit_price": "6.00", "conditions": map[string]any{"min_total_tokens": 0}},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/api/console/models/"+modelID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", modelID.String())
	w := httptest.NewRecorder()
	HandleUpdateModel(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	// GET shows the tiered rows.
	req = httptest.NewRequest(http.MethodGet, "/api/console/models/"+modelID.String(), nil)
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", modelID.String())
	w = httptest.NewRecorder()
	HandleGetModel(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", w.Code)
	}
	var getResp struct {
		Data struct {
			Pricings []struct {
				Dimension  string         `json:"dimension"`
				UnitPrice  string         `json:"unit_price"`
				Conditions map[string]any `json:"conditions"`
			} `json:"pricings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	foundTier := false
	for _, p := range getResp.Data.Pricings {
		if p.Dimension == "input" && trimDecimalPrice(p.UnitPrice) == "2.00" {
			foundTier = true
			if p.Conditions["max_total_tokens"] != float64(200000) {
				t.Errorf("input conditions = %v, want max_total_tokens 200000", p.Conditions)
			}
		}
	}
	if !foundTier {
		t.Error("tiered input row missing after update")
	}

	// Clearing the tier (conditions omitted) must remove the conditions so the
	// row behaves as the generic price again.
	clearBody := map[string]interface{}{
		"pricings": []map[string]interface{}{
			{"dimension": "input", "unit_name": "1M tokens", "unit_price": "2.50"},
		},
	}
	clearBytes, _ := json.Marshal(clearBody)
	req = httptest.NewRequest(http.MethodPut, "/api/console/models/"+modelID.String(), bytes.NewReader(clearBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInModelsContext(req, uuid.New().String())
	req = chiRouteCtx(req, "id", modelID.String())
	w = httptest.NewRecorder()
	HandleUpdateModel(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("clear status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var condText string
	err = a.Pool.QueryRow(context.Background(),
		`SELECT COALESCE(conditions, '{}'::jsonb)::text FROM model_pricing
		 WHERE model_id = $1 AND pricing_dimension = 'input' AND price_type = 'sell'`, modelID).Scan(&condText)
	if err != nil {
		t.Fatalf("query cleared conditions: %v", err)
	}
	if condText != "{}" {
		t.Errorf("cleared input conditions = %s, want {}", condText)
	}
}

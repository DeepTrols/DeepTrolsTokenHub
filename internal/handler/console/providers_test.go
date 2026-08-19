package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/apikey"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// appForProviderTest creates a minimal App with a pool for provider tests.
func appForProviderTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-provider-32byt",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Users:   user.NewPostgresRepository(pool),
		APIKeys: apikey.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedUserForProviderTest creates a user with a given role.
func seedUserForProviderTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
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
		t.Fatalf("seedUserForProviderTest: create: %v", err)
	}
	return u
}

// seedModelForProviderTest creates a model and returns its ID.
func seedModelForProviderTest(t *testing.T, a *app.App, code, provider string) uuid.UUID {
	t.Helper()
	modelID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
		 VALUES ($1, $2, $3, 'chat', $2, 'active', 'GA', $4, $4)`,
		modelID, code, provider, now,
	)
	if err != nil {
		t.Fatalf("seedModelForProviderTest: %v", err)
	}
	return modelID
}

// setAdminContext adds user_id and role="admin" to the request context.
func setAdminContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxRoleKey, "admin")
	return r.WithContext(ctx)
}

// setUserContext adds user_id without admin role to the request context.
func setUserContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxRoleKey, "user")
	return r.WithContext(ctx)
}

// =============================================================================
// HandleListProviders Tests
// =============================================================================

func TestHandleListProviders_NoAuth(t *testing.T) {
	a := appForProviderTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	w := httptest.NewRecorder()

	handler := HandleListProviders(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListProviders_NotAdmin(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "notadmin@example.com", "pass123", "Not Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	req = setAdminContext(req, user.ID.String())
	// Override role to "user" to simulate non-admin
	ctx := req.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxRoleKey, "user")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler := HandleListProviders(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleListProviders_EmptyList(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-empty@example.com", "pass", "Admin Empty")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListProviders(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("data field is not an array")
	}
	if len(data) != 0 {
		t.Errorf("expected empty array, got %d items", len(data))
	}
}

func TestHandleListProviders_ReturnsProviders(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-list@example.com", "pass", "Admin List")
	modelID := seedModelForProviderTest(t, a, "gpt-4o", "openai")

	// Seed a provider channel + instance directly
	channelID := uuid.New()
	instanceID := uuid.New()
	now := time.Now().UTC()
	ctx := context.Background()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', $4, $4)`,
		channelID, "OpenAI Production", modelID, now,
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	config := map[string]interface{}{"api_key": "sk-proj-1234abcd", "provider": "openai", "display_name": "OpenAI Production"}
	configJSON, _ := json.Marshal(config)
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'openai', $3, $4, 'active', $5, $5)`,
		instanceID, channelID, "https://api.openai.com/v1", configJSON, now,
	)
	if err != nil {
		t.Fatalf("seed channel_instance: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListProviders(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []providerResponse `json:"data"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data))
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}

	p := resp.Data[0]
	if p.ID != channelID.String() {
		t.Errorf("id = %s, want %s", p.ID, channelID.String())
	}
	if p.Name != "OpenAI Production" {
		t.Errorf("name = %s, want 'OpenAI Production'", p.Name)
	}
	if p.Provider != "openai" {
		t.Errorf("provider = %s, want 'openai'", p.Provider)
	}
	if p.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base_url = %s, want 'https://api.openai.com/v1'", p.BaseURL)
	}
	if p.MaskedKey != "****abcd" {
		t.Errorf("masked_key = %s, want '****abcd'", p.MaskedKey)
	}
	if p.Status != "active" {
		t.Errorf("status = %s, want 'active'", p.Status)
	}
}

func TestHandleListProviders_MasksShortKeys(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-short@example.com", "pass", "Admin Short")
	modelID := seedModelForProviderTest(t, a, "claude-sonnet", "anthropic")

	channelID := uuid.New()
	instanceID := uuid.New()
	now := time.Now().UTC()
	ctx := context.Background()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', $4, $4)`,
		channelID, "Anthropic Test", modelID, now,
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	// Short API key with only 3 characters
	config := map[string]interface{}{"api_key": "abc", "provider": "anthropic"}
	configJSON, _ := json.Marshal(config)
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'anthropic', 'https://api.anthropic.com', $3, 'active', $4, $4)`,
		instanceID, channelID, configJSON, now,
	)
	if err != nil {
		t.Fatalf("seed channel_instance: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListProviders(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []providerResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data))
	}

	// For a 3-char key, all chars are shown via "****" masking (since len <= 4)
	if resp.Data[0].MaskedKey == "" {
		t.Fatal("masked_key should not be empty for short keys")
	}
}

func TestHandleListProviders_MissingInstance(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-no-instance@example.com", "pass", "Admin No Instance")
	modelID := seedModelForProviderTest(t, a, "gpt-3.5", "openai")

	channelID := uuid.New()
	now := time.Now().UTC()

	// Channel with no channel_instance
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', $4, $4)`,
		channelID, "No Instance Provider", modelID, now,
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/providers", nil)
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListProviders(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Channel without instance should still appear with empty fields
	var resp struct {
		Data []providerResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "No Instance Provider" {
		t.Errorf("name = %s, want 'No Instance Provider'", resp.Data[0].Name)
	}
}

// =============================================================================
// HandleCreateProvider Tests
// =============================================================================

func TestHandleCreateProvider_NoAuth(t *testing.T) {
	a := appForProviderTest(t)

	body := map[string]string{"name": "Test Provider", "provider": "openai", "base_url": "https://api.openai.com/v1", "api_key": "sk-test"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleCreateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateProvider_NotAdmin(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "create-notadmin@example.com", "pass", "Not Admin")

	body := map[string]string{"name": "Test", "provider": "openai", "base_url": "https://api.openai.com/v1", "api_key": "sk-test"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleCreateProvider_InvalidBody(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-badbody@example.com", "pass", "Admin Bad Body")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateProvider_MissingRequiredFields(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-missing@example.com", "pass", "Admin Missing")

	tests := []struct {
		name string
		body map[string]string
	}{
		{"empty name", map[string]string{"name": "", "provider": "openai", "base_url": "https://api.openai.com/v1", "api_key": "sk-test"}},
		{"missing provider", map[string]string{"name": "Test", "provider": "", "base_url": "https://api.openai.com/v1", "api_key": "sk-test"}},
		{"missing api_key", map[string]string{"name": "Test", "provider": "openai", "base_url": "https://api.openai.com/v1", "api_key": ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setAdminContext(req, user.ID.String())
			w := httptest.NewRecorder()

			handler := HandleCreateProvider(a)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

func TestHandleCreateProvider_NoModelsExist(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-nomodels@example.com", "pass", "Admin No Models")

	// Stub model discovery to return no models (as if upstream has none / API
	// key is invalid) — the credential must still be saved so it appears in UI.
	orig := discoverModelsFn
	discoverModelsFn = func(provider, baseURL, apiKey string) ([]modelRef, error) {
		return nil, nil
	}
	defer func() { discoverModelsFn = orig }()

	body := map[string]string{
		"name":     "OpenAI Production",
		"provider": "openai",
		"base_url": "https://api.openai.com/v1",
		"api_key":  "sk-proj-test12345678",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateProvider(a)
	handler.ServeHTTP(w, req)

	// The credential is saved even when discovery finds no models.
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestHandleCreateProvider_Success(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-create@example.com", "pass", "Admin Create")

	// Stub discovery to return one matching model.
	orig := discoverModelsFn
	discoverModelsFn = func(provider, baseURL, apiKey string) ([]modelRef, error) {
		return []modelRef{{ID: "gpt-4o"}}, nil
	}
	defer func() { discoverModelsFn = orig }()

	body := map[string]string{
		"name":     "OpenAI Production",
		"provider": "openai",
		"base_url": "https://api.openai.com/v1",
		"api_key":  "sk-proj-deadbeef12345678",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Provider  string `json:"provider"`
		BaseURL   string `json:"base_url"`
		MaskedKey string `json:"masked_key"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Name != "OpenAI Production" {
		t.Errorf("name = %s, want 'OpenAI Production'", resp.Name)
	}
	if resp.Provider != "openai" {
		t.Errorf("provider = %s, want 'openai'", resp.Provider)
	}
	if resp.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base_url = %s, want 'https://api.openai.com/v1'", resp.BaseURL)
	}
	if resp.Status != "active" {
		t.Errorf("status = %s, want 'active'", resp.Status)
	}

	// Verify database state: channel + channel_instance exist for the created model.
	var count int
	err := a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM channels WHERE name LIKE 'OpenAI Production%'`,
	).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected 1 channel in DB, got %d (err: %v)", count, err)
	}

	err = a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM channel_instances WHERE channel_id IN (SELECT id FROM channels WHERE name LIKE 'OpenAI Production%')`,
	).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected 1 channel_instance in DB, got %d (err: %v)", count, err)
	}

	// Verify API key is stored in config JSONB (not plaintext in resp).
	var configJSON []byte
	err = a.Pool.QueryRow(context.Background(),
		`SELECT config FROM channel_instances WHERE channel_id IN (SELECT id FROM channels WHERE name LIKE 'OpenAI Production%')`,
	).Scan(&configJSON)
	if err != nil {
		t.Fatalf("query config: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if config["api_key"] != "sk-proj-deadbeef12345678" {
		t.Errorf("stored api_key = %s, want 'sk-proj-deadbeef12345678'", config["api_key"])
	}
}

func TestHandleCreateProvider_WithDefaultBaseURL(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-default-url@example.com", "pass", "Admin Default URL")

	orig := discoverModelsFn
	discoverModelsFn = func(provider, baseURL, apiKey string) ([]modelRef, error) {
		return []modelRef{{ID: "deepseek/deepseek-v3"}}, nil
	}
	defer func() { discoverModelsFn = orig }()

	body := map[string]string{
		"name":     "DeepSeek Dev",
		"provider": "deepseek",
		"api_key":  "sk-deepseek-test12345678",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/providers", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Verify base_url was defaulted for deepseek by looking up the created
	// instance in the DB (the create response is a summary, not a single id).
	// Note: the stored base_url is WITHOUT /v1 — the gateway executor appends
	// "/v1/chat/completions" at call time.
	var baseURL string
	err := a.Pool.QueryRow(context.Background(),
		`SELECT ci.base_url FROM channel_instances ci
		 JOIN channels ch ON ch.id = ci.channel_id
		 WHERE ch.name LIKE 'DeepSeek%' LIMIT 1`,
	).Scan(&baseURL)
	if err != nil {
		t.Fatalf("query base_url: %v", err)
	}
	if baseURL != "https://api.deepseek.com" {
		t.Errorf("base_url = %s, want 'https://api.deepseek.com'", baseURL)
	}
}

// =============================================================================
// HandleUpdateProvider Tests
// =============================================================================

func TestHandleUpdateProvider_NoAuth(t *testing.T) {
	a := appForProviderTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleUpdateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUpdateProvider_NotFound(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-update-404@example.com", "pass", "Admin Update 404")

	nonExistentID := uuid.New().String()
	body := map[string]string{"name": "Updated Name"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/"+nonExistentID, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleUpdateProvider_Success(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-update@example.com", "pass", "Admin Update")
	modelID := seedModelForProviderTest(t, a, "gpt-4o", "openai")

	// Seed existing provider
	channelID := uuid.New()
	instanceID := uuid.New()
	now := time.Now().UTC()
	ctx := context.Background()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', $4, $4)`,
		channelID, "OpenAI Old Name", modelID, now,
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	config := map[string]interface{}{"api_key": "sk-old-key", "provider": "openai"}
	configJSON, _ := json.Marshal(config)
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'openai', 'https://api.openai.com/v1', $3, 'active', $4, $4)`,
		instanceID, channelID, configJSON, now,
	)
	if err != nil {
		t.Fatalf("seed channel_instance: %v", err)
	}

	// Update name and api_key
	body := map[string]string{
		"name":    "OpenAI Updated Name",
		"api_key": "sk-new-key-abcdef",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/"+channelID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", channelID.String())
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify channel name updated
	var name string
	err = a.Pool.QueryRow(ctx, `SELECT name FROM channels WHERE id = $1`, channelID).Scan(&name)
	if err != nil {
		t.Fatalf("query channel: %v", err)
	}
	if name != "OpenAI Updated Name" {
		t.Errorf("name = %s, want 'OpenAI Updated Name'", name)
	}

	// Verify API key updated in config
	var configJSONUpdated []byte
	err = a.Pool.QueryRow(ctx,
		`SELECT config FROM channel_instances WHERE channel_id = $1`, channelID,
	).Scan(&configJSONUpdated)
	if err != nil {
		t.Fatalf("query config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(configJSONUpdated, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg["api_key"] != "sk-new-key-abcdef" {
		t.Errorf("stored api_key = %s, want 'sk-new-key-abcdef'", cfg["api_key"])
	}
}

func TestHandleUpdateProvider_UpdateBaseURL(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-update-url@example.com", "pass", "Admin Update URL")
	modelID := seedModelForProviderTest(t, a, "claude-sonnet", "anthropic")

	channelID := uuid.New()
	instanceID := uuid.New()
	now := time.Now().UTC()
	ctx := context.Background()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', $4, $4)`,
		channelID, "Anthropic", modelID, now,
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	config := map[string]interface{}{"api_key": "sk-ant-old", "provider": "anthropic"}
	configJSON, _ := json.Marshal(config)
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'anthropic', 'https://api.anthropic.com', $3, 'active', $4, $4)`,
		instanceID, channelID, configJSON, now,
	)
	if err != nil {
		t.Fatalf("seed channel_instance: %v", err)
	}

	body := map[string]string{"base_url": "https://custom-anthropic.example.com/v1"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/"+channelID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", channelID.String())
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var baseURL string
	err = a.Pool.QueryRow(ctx,
		`SELECT base_url FROM channel_instances WHERE channel_id = $1`, channelID,
	).Scan(&baseURL)
	if err != nil {
		t.Fatalf("query base_url: %v", err)
	}
	if baseURL != "https://custom-anthropic.example.com/v1" {
		t.Errorf("base_url = %s, want 'https://custom-anthropic.example.com/v1'", baseURL)
	}
}

func TestHandleUpdateProvider_InvalidBody(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-update-bad@example.com", "pass", "Admin Update Bad")
	modelID := seedModelForProviderTest(t, a, "gpt-4o", "openai")

	channelID := uuid.New()
	now := time.Now().UTC()

	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', $4, $4)`,
		channelID, "Test Provider", modelID, now,
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/admin/providers/"+channelID.String(), bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", channelID.String())
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// HandleDeleteProvider Tests
// =============================================================================

func TestHandleDeleteProvider_NoAuth(t *testing.T) {
	a := appForProviderTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/providers/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleDeleteProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteProvider_NotFound(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-del-404@example.com", "pass", "Admin Del 404")

	nonExistentID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/providers/"+nonExistentID, nil)
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteProvider_InvalidID(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-del-invalid@example.com", "pass", "Admin Del Invalid")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/providers/not-a-uuid", nil)
	req = chiRouteCtx(req, "id", "not-a-uuid")
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteProvider_Success(t *testing.T) {
	a := appForProviderTest(t)
	user := seedUserForProviderTest(t, a, "admin-delete@example.com", "pass", "Admin Delete")
	modelID := seedModelForProviderTest(t, a, "gpt-4o", "openai")

	channelID := uuid.New()
	instanceID := uuid.New()
	now := time.Now().UTC()
	ctx := context.Background()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', $4, $4)`,
		channelID, "To Be Deleted", modelID, now,
	)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	config := map[string]interface{}{"api_key": "sk-to-delete", "provider": "openai"}
	configJSON, _ := json.Marshal(config)
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'openai', 'https://api.openai.com/v1', $3, 'active', $4, $4)`,
		instanceID, channelID, configJSON, now,
	)
	if err != nil {
		t.Fatalf("seed channel_instance: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/providers/"+channelID.String(), nil)
	req = chiRouteCtx(req, "id", channelID.String())
	req = setAdminContext(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteProvider(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify soft-delete: channel status should be 'inactive'
	var status string
	err = a.Pool.QueryRow(ctx, `SELECT status FROM channels WHERE id = $1`, channelID).Scan(&status)
	if err != nil {
		t.Fatalf("query channel: %v", err)
	}
	if status != "inactive" {
		t.Errorf("channel status = %s, want 'inactive'", status)
	}

	// Verify instance is also inactive
	err = a.Pool.QueryRow(ctx, `SELECT status FROM channel_instances WHERE channel_id = $1`, channelID).Scan(&status)
	if err != nil {
		t.Fatalf("query channel_instance: %v", err)
	}
	if status != "inactive" {
		t.Errorf("instance status = %s, want 'inactive'", status)
	}
}

// =============================================================================
// API Key Masking Tests
// =============================================================================

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"sk-proj-deadbeef12345678", "****5678"},
		{"sk-ant-api03-secret", "****cret"},
		{"abc", "****abc"},
		{"ab", "****ab"},
		{"a", "****a"},
		{"", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := maskAPIKey(tt.key)
			if got != tt.want {
				t.Errorf("maskAPIKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

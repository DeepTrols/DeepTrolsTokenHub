package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/apikey"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/usage"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
)

// appForUsageTest creates a minimal App with repos wired for usage tests.
func appForUsageTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-usage-32-byte",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Users:   user.NewPostgresRepository(pool),
		APIKeys: apikey.NewPostgresRepository(pool),
		Usage:   usage.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedUserForUsageTest creates a user with bcrypt hash.
func seedUserForUsageTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
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
		t.Fatalf("seedUserForUsageTest: create: %v", err)
	}
	return u
}

// setUserInUsageContext adds a user ID to the request context.
func setUserInUsageContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	return r.WithContext(ctx)
}

// seedAPIKeyForUsage creates an API key for the user (needed for usage_log FK).
func seedAPIKeyForUsage(t *testing.T, a *app.App, userID uuid.UUID) *domain.APIKey {
	t.Helper()
	key := domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash-usage-test-" + uuid.New().String()[:8],
		MaskedKey:       "dt-sk-****test",
		Name:            "Usage Test Key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := a.APIKeys.Create(context.Background(), &key); err != nil {
		t.Fatalf("seedAPIKeyForUsage: create: %v", err)
	}
	return &key
}

// =============================================================================
// HandleListUsage Tests
// =============================================================================

func TestHandleListUsage_NoAuth(t *testing.T) {
	a := appForUsageTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/usage", nil)
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListUsage_EmptyList(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-empty@example.com", "pass", "Empty Usage")

	req := httptest.NewRequest(http.MethodGet, "/api/console/usage", nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
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

func TestHandleListUsage_WithUsageLogs(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-has@example.com", "pass", "Has Usage")
	_ = seedAPIKeyForUsage(t, a, seedUser.ID)

	// Seed usage logs directly
	apiKey := seedAPIKeyForUsage(t, a, seedUser.ID)
	log1 := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		APIKeyID:        apiKey.ID,
		RequestID:       "req-abc123",
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		UsageNormalized: map[string]interface{}{"input_tokens": float64(150), "output_tokens": float64(300)},
		ListCost:        decimal.RequireFromString("0.0035"),
		FinalCost:       decimal.RequireFromString("0.0035"),
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	log2 := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		APIKeyID:        apiKey.ID,
		RequestID:       "req-def456",
		RequestType:     "chat",
		PublicModelCode: "claude-sonnet",
		UsageSource:     domain.UsageSourceUpstream,
		UsageNormalized: map[string]interface{}{"input_tokens": float64(50), "output_tokens": float64(100)},
		ListCost:        decimal.RequireFromString("0.0010"),
		FinalCost:       decimal.RequireFromString("0.0010"),
		Status:          domain.UsageLogStatusFailed,
		CreatedAt:       time.Now().UTC(),
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &log1); err != nil {
		t.Fatalf("create log1: %v", err)
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &log2); err != nil {
		t.Fatalf("create log2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/usage", nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			ID           string `json:"id"`
			Model        string `json:"model"`
			RequestID    string `json:"request_id"`
			Status       string `json:"status"`
			APIKeyName   string `json:"api_key_name"`
			InputTokens  int    `json:"input_tokens"`
			OutputTokens int    `json:"output_tokens"`
			Cost         string `json:"cost"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 usage logs, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	// Verify log data
	foundCompleted, foundFailed := false, false
	for _, l := range resp.Data {
		if l.Model == "gpt-4o" && l.Status == "completed" {
			foundCompleted = true
			if l.InputTokens != 150 {
				t.Errorf("gpt-4o input_tokens = %d, want 150", l.InputTokens)
			}
			if l.OutputTokens != 300 {
				t.Errorf("gpt-4o output_tokens = %d, want 300", l.OutputTokens)
			}
			if l.APIKeyName != "Usage Test Key" {
				t.Errorf("api_key_name = %q, want %q", l.APIKeyName, "Usage Test Key")
			}
		}
		if l.Model == "claude-sonnet" && l.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundCompleted {
		t.Error("completed gpt-4o log not found")
	}
	if !foundFailed {
		t.Error("failed claude-sonnet log not found")
	}
}

func TestHandleListUsage_FilterByModel(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-filter@example.com", "pass", "Filter Usage")
	apiKey := seedAPIKeyForUsage(t, a, seedUser.ID)

	// Create logs for different models
	log1 := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		APIKeyID:        apiKey.ID,
		RequestID:       "req-model-a",
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		UsageNormalized: map[string]interface{}{"input_tokens": float64(10)},
		ListCost:        decimal.RequireFromString("0.001"),
		FinalCost:       decimal.RequireFromString("0.001"),
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	log2 := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		APIKeyID:        apiKey.ID,
		RequestID:       "req-model-b",
		RequestType:     "chat",
		PublicModelCode: "claude-sonnet",
		UsageSource:     domain.UsageSourceUpstream,
		UsageNormalized: map[string]interface{}{"input_tokens": float64(20)},
		ListCost:        decimal.RequireFromString("0.002"),
		FinalCost:       decimal.RequireFromString("0.002"),
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &log1); err != nil {
		t.Fatalf("create log1: %v", err)
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &log2); err != nil {
		t.Fatalf("create log2: %v", err)
	}

	// Filter by gpt-4o
	req := httptest.NewRequest(http.MethodGet, "/api/console/usage?model=gpt-4o", nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			Model string `json:"model"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 filtered log, got %d", len(resp.Data))
	}
	if resp.Data[0].Model != "gpt-4o" {
		t.Errorf("model = %s, want 'gpt-4o'", resp.Data[0].Model)
	}
}

func TestHandleListUsage_FilterByStatus(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-status@example.com", "pass", "Status Usage")
	apiKey := seedAPIKeyForUsage(t, a, seedUser.ID)

	log1 := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		APIKeyID:        apiKey.ID,
		RequestID:       "req-completed",
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.RequireFromString("0.001"),
		FinalCost:       decimal.RequireFromString("0.001"),
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	log2 := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		APIKeyID:        apiKey.ID,
		RequestID:       "req-failed",
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.RequireFromString("0.001"),
		FinalCost:       decimal.RequireFromString("0.001"),
		Status:          domain.UsageLogStatusFailed,
		CreatedAt:       time.Now().UTC(),
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &log1); err != nil {
		t.Fatalf("create log1: %v", err)
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &log2); err != nil {
		t.Fatalf("create log2: %v", err)
	}

	// Filter by status=failed
	req := httptest.NewRequest(http.MethodGet, "/api/console/usage?status=failed", nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 failed log, got %d", len(resp.Data))
	}
	if resp.Data[0].Status != "failed" {
		t.Errorf("status = %s, want 'failed'", resp.Data[0].Status)
	}
}

func TestHandleListUsage_OnlyOwnUsage(t *testing.T) {
	a := appForUsageTest(t)
	userA := seedUserForUsageTest(t, a, "usage-onlyA@example.com", "passA", "Only A")
	userB := seedUserForUsageTest(t, a, "usage-onlyB@example.com", "passB", "Only B")
	apiKeyA := seedAPIKeyForUsage(t, a, userA.ID)
	apiKeyB := seedAPIKeyForUsage(t, a, userB.ID)

	logA := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          userA.ID,
		APIKeyID:        apiKeyA.ID,
		RequestID:       "req-user-a",
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.RequireFromString("0.001"),
		FinalCost:       decimal.RequireFromString("0.001"),
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	logB := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          userB.ID,
		APIKeyID:        apiKeyB.ID,
		RequestID:       "req-user-b",
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.RequireFromString("0.001"),
		FinalCost:       decimal.RequireFromString("0.001"),
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &logA); err != nil {
		t.Fatalf("create logA: %v", err)
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &logB); err != nil {
		t.Fatalf("create logB: %v", err)
	}

	// Request as user A
	req := httptest.NewRequest(http.MethodGet, "/api/console/usage", nil)
	req = setUserInUsageContext(req, userA.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			RequestID string `json:"request_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 log for user A, got %d", len(resp.Data))
	}
	if resp.Data[0].RequestID != "req-user-a" {
		t.Errorf("request_id = %s, want 'req-user-a'", resp.Data[0].RequestID)
	}
}

func TestHandleListUsage_FilterByAPIKey(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-apikey@example.com", "pass", "APIKey Usage")
	keyA := seedAPIKeyForUsage(t, a, seedUser.ID)
	keyB := seedAPIKeyForUsage(t, a, seedUser.ID)

	mkLog := func(reqID string, keyID uuid.UUID) domain.UsageLog {
		return domain.UsageLog{
			ID:              uuid.New(),
			UserID:          seedUser.ID,
			APIKeyID:        keyID,
			RequestID:       reqID,
			RequestType:     "chat",
			PublicModelCode: "gpt-4o",
			UsageSource:     domain.UsageSourceUpstream,
			ListCost:        decimal.RequireFromString("0.001"),
			FinalCost:       decimal.RequireFromString("0.001"),
			Status:          domain.UsageLogStatusCompleted,
			CreatedAt:       time.Now().UTC(),
		}
	}
	for _, lg := range []domain.UsageLog{
		mkLog("req-api-a", keyA.ID),
		mkLog("req-api-b", keyB.ID),
	} {
		if err := a.Usage.CreateUsageLog(context.Background(), &lg); err != nil {
			t.Fatalf("create log: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/usage?api_key_id="+keyB.ID.String(), nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			RequestID string `json:"request_id"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 log filtered by api_key_id, got %d", len(resp.Data))
	}
	if resp.Data[0].RequestID != "req-api-b" {
		t.Errorf("request_id = %s, want 'req-api-b'", resp.Data[0].RequestID)
	}
	if resp.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Total)
	}
}

func TestHandleListUsage_InvalidAPIKeyFilter(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-badkey@example.com", "pass", "Bad Key Usage")

	req := httptest.NewRequest(http.MethodGet, "/api/console/usage?api_key_id=not-a-uuid", nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleListUsage_FilterByFrom(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-from@example.com", "pass", "From Usage")
	apiKey := seedAPIKeyForUsage(t, a, seedUser.ID)

	// Seed a log 48h in the past.
	log := domain.UsageLog{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		APIKeyID:        apiKey.ID,
		RequestID:       "req-from-past",
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.RequireFromString("0.001"),
		FinalCost:       decimal.RequireFromString("0.001"),
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC().Add(-48 * time.Hour),
	}
	if err := a.Usage.CreateUsageLog(context.Background(), &log); err != nil {
		t.Fatalf("create log: %v", err)
	}

	// from = now excludes the 48h-old log.
	from := url.QueryEscape(time.Now().UTC().Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, "/api/console/usage?from="+from, nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
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
		t.Errorf("expected 0 logs after from filter, got %d", len(resp.Data))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}

func TestHandleListUsage_InvalidDateFilter(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-badfrom@example.com", "pass", "Bad From Usage")

	for _, qs := range []string{"from=not-a-date", "to=not-a-date"} {
		req := httptest.NewRequest(http.MethodGet, "/api/console/usage?"+qs, nil)
		req = setUserInUsageContext(req, seedUser.ID.String())
		w := httptest.NewRecorder()

		handler := HandleListUsage(a)
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("?%s status = %d, want %d", qs, w.Code, http.StatusBadRequest)
		}
	}
}

func TestHandleListUsage_APIKeyFilterNoCrossUserLeak(t *testing.T) {
	a := appForUsageTest(t)
	userA := seedUserForUsageTest(t, a, "usage-leakA@example.com", "passA", "Leak A")
	userB := seedUserForUsageTest(t, a, "usage-leakB@example.com", "passB", "Leak B")
	keyA := seedAPIKeyForUsage(t, a, userA.ID)
	keyB := seedAPIKeyForUsage(t, a, userB.ID)

	for _, lg := range []domain.UsageLog{
		{
			ID: uuid.New(), UserID: userA.ID, APIKeyID: keyA.ID, RequestID: "req-leak-a",
			RequestType: "chat", PublicModelCode: "gpt-4o", UsageSource: domain.UsageSourceUpstream,
			ListCost: decimal.RequireFromString("0.001"), FinalCost: decimal.RequireFromString("0.001"),
			Status: domain.UsageLogStatusCompleted, CreatedAt: time.Now().UTC(),
		},
		{
			ID: uuid.New(), UserID: userB.ID, APIKeyID: keyB.ID, RequestID: "req-leak-b",
			RequestType: "chat", PublicModelCode: "gpt-4o", UsageSource: domain.UsageSourceUpstream,
			ListCost: decimal.RequireFromString("0.001"), FinalCost: decimal.RequireFromString("0.001"),
			Status: domain.UsageLogStatusCompleted, CreatedAt: time.Now().UTC(),
		},
	} {
		if err := a.Usage.CreateUsageLog(context.Background(), &lg); err != nil {
			t.Fatalf("create log: %v", err)
		}
	}

	// User A filters by their own key, but must never see user B's log even
	// though a cross-user api_key_id would be passed.
	req := httptest.NewRequest(http.MethodGet, "/api/console/usage?api_key_id="+keyB.ID.String(), nil)
	req = setUserInUsageContext(req, userA.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			RequestID string `json:"request_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Fatalf("expected 0 logs, got %d (cross-user leak)", len(resp.Data))
	}
}

func TestHandleListUsage_Pagination(t *testing.T) {
	a := appForUsageTest(t)
	seedUser := seedUserForUsageTest(t, a, "usage-page@example.com", "pass", "Page Usage")
	apiKey := seedAPIKeyForUsage(t, a, seedUser.ID)

	// Create 5 logs
	for i := 0; i < 5; i++ {
		log := domain.UsageLog{
			ID:              uuid.New(),
			UserID:          seedUser.ID,
			APIKeyID:        apiKey.ID,
			RequestID:       "req-page-" + string(rune('a'+i)),
			RequestType:     "chat",
			PublicModelCode: "gpt-4o",
			UsageSource:     domain.UsageSourceUpstream,
			ListCost:        decimal.RequireFromString("0.001"),
			FinalCost:       decimal.RequireFromString("0.001"),
			Status:          domain.UsageLogStatusCompleted,
			CreatedAt:       time.Now().UTC(),
		}
		if err := a.Usage.CreateUsageLog(context.Background(), &log); err != nil {
			t.Fatalf("create log %d: %v", i, err)
		}
	}

	// Request with limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/console/usage?limit=2&offset=0", nil)
	req = setUserInUsageContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListUsage(a)
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
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 usage logs (limit=2), got %d", len(resp.Data))
	}
}

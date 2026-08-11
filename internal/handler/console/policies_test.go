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
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// =============================================================================
// Test helpers
// =============================================================================

// appForPoliciesTest creates a minimal App with a pool for policy tests.
func appForPoliciesTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-policies-32-byte",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Users:   user.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedUserForPoliciesTest creates a user with bcrypt hash.
func seedUserForPoliciesTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
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
		t.Fatalf("seedUserForPoliciesTest: create: %v", err)
	}
	return u
}

// seedModelForPoliciesTest creates a model and returns its ID.
func seedModelForPoliciesTest(t *testing.T, a *app.App, code, provider string) uuid.UUID {
	t.Helper()
	modelID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
		 VALUES ($1, $2, $3, 'chat', $2, 'active', 'GA', $4, $4)`,
		modelID, code, provider, now,
	)
	if err != nil {
		t.Fatalf("seedModelForPoliciesTest: %v", err)
	}
	return modelID
}

// seedChannelForPoliciesTest inserts a channel and returns its ID.
func seedChannelForPoliciesTest(t *testing.T, a *app.App, name string, modelID uuid.UUID) uuid.UUID {
	t.Helper()
	channelID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO channels (id, name, model_id, pool_type, status, health_score, health_status, weight, max_concurrency, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', 100, 'healthy', 100, 10, $4, $4)`,
		channelID, name, modelID, now,
	)
	if err != nil {
		t.Fatalf("seedChannelForPoliciesTest: %v", err)
	}
	return channelID
}

// seedPolicyForTest inserts a route_policy and returns its ID.
func seedPolicyForTest(t *testing.T, a *app.App, name string, modelID *uuid.UUID, channelIDs []uuid.UUID) uuid.UUID {
	t.Helper()
	policyID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO route_policies (id, name, user_level, model_id, priority, candidate_channel_ids, fallback_policy, is_active, created_at, updated_at)
		 VALUES ($1, $2, 'basic', $3, 10, $4, 'disabled', true, $5, $5)`,
		policyID, name, modelID, channelIDs, now,
	)
	if err != nil {
		t.Fatalf("seedPolicyForTest: %v", err)
	}
	return policyID
}

// =============================================================================
// HandleListPolicies Tests
// =============================================================================

func TestHandleListPolicies_NoAuth(t *testing.T) {
	a := appForPoliciesTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/policies", nil)
	w := httptest.NewRecorder()

	handler := HandleListPolicies(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListPolicies_NotAdmin(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-list-notadmin@example.com", "pass123", "Not Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/policies", nil)
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListPolicies(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleListPolicies_EmptyList(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-list-empty@example.com", "pass", "Admin Empty")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/policies", nil)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListPolicies(a)
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

func TestHandleListPolicies_ReturnsPolicies(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-list@example.com", "pass", "Admin List")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	ch1 := seedChannelForPoliciesTest(t, a, "OpenAI Prod", modelID)
	ch2 := seedChannelForPoliciesTest(t, a, "OpenAI Staging", modelID)

	p1 := seedPolicyForTest(t, a, "Policy A", &modelID, []uuid.UUID{ch1, ch2})
	p2 := seedPolicyForTest(t, a, "Policy B", nil, []uuid.UUID{ch1})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/policies", nil)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListPolicies(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []policyResponse `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	foundP1, foundP2 := false, false
	for _, p := range resp.Data {
		switch p.ID {
		case p1.String():
			foundP1 = true
			if p.Name != "Policy A" {
				t.Errorf("p1 name = %s, want 'Policy A'", p.Name)
			}
			if p.ModelID == nil || *p.ModelID != modelID.String() {
				t.Errorf("p1 model_id = %v, want %s", p.ModelID, modelID.String())
			}
		case p2.String():
			foundP2 = true
			if p.Name != "Policy B" {
				t.Errorf("p2 name = %s, want 'Policy B'", p.Name)
			}
			if p.ModelID != nil {
				t.Errorf("p2 model_id = %v, want nil", p.ModelID)
			}
		}
	}
	if !foundP1 {
		t.Errorf("policy %s not found in response", p1)
	}
	if !foundP2 {
		t.Errorf("policy %s not found in response", p2)
	}
}

func TestHandleListPolicies_ShowsChannelNames(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-list-names@example.com", "pass", "Admin Names")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	ch1 := seedChannelForPoliciesTest(t, a, "OpenAI Prod", modelID)
	ch2 := seedChannelForPoliciesTest(t, a, "Anthropic Staging", modelID)

	seedPolicyForTest(t, a, "Named Channel Policy", &modelID, []uuid.UUID{ch1, ch2})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/policies", nil)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListPolicies(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []policyResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(resp.Data))
	}

	p := resp.Data[0]
	if len(p.CandidateChannelNames) != 2 {
		t.Errorf("expected 2 channel names, got %d: %v", len(p.CandidateChannelNames), p.CandidateChannelNames)
	}
}

// =============================================================================
// HandleCreatePolicy Tests
// =============================================================================

func TestHandleCreatePolicy_NoAuth(t *testing.T) {
	a := appForPoliciesTest(t)

	body := map[string]interface{}{
		"name":                  "Test Policy",
		"user_level":            "basic",
		"priority":              10,
		"candidate_channel_ids": []string{},
		"fallback_policy":       "disabled",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreatePolicy_NotAdmin(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-notadmin@example.com", "pass", "Not Admin")

	body := map[string]interface{}{"name": "Test", "user_level": "basic", "priority": 10, "candidate_channel_ids": []string{}, "fallback_policy": "disabled"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleCreatePolicy_InvalidBody(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-badbody@example.com", "pass", "Admin Bad Body")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreatePolicy_MissingRequiredFields(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-missing@example.com", "pass", "Admin Missing")

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"empty name", map[string]interface{}{"name": "", "user_level": "basic", "priority": 10, "candidate_channel_ids": []string{}, "fallback_policy": "disabled"}},
		{"empty user_level", map[string]interface{}{"name": "Test", "user_level": "", "priority": 10, "candidate_channel_ids": []string{}, "fallback_policy": "disabled"}},
		{"empty fallback_policy", map[string]interface{}{"name": "Test", "user_level": "basic", "priority": 10, "candidate_channel_ids": []string{}, "fallback_policy": ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setAdminCtx(req, user.ID.String())
			w := httptest.NewRecorder()

			handler := HandleCreatePolicy(a)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

func TestHandleCreatePolicy_InvalidFallbackPolicy(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-badfp@example.com", "pass", "Admin Bad Fallback")

	chID := seedChannelForPoliciesTest(t, a, "Test Channel", seedModelForPoliciesTest(t, a, "gpt-4o", "openai"))

	body := map[string]interface{}{
		"name":                  "Test Policy",
		"user_level":            "basic",
		"priority":              10,
		"candidate_channel_ids": []string{chID.String()},
		"fallback_policy":       "invalid_value",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleCreatePolicy_Success(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create@example.com", "pass", "Admin Create")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	ch1 := seedChannelForPoliciesTest(t, a, "OpenAI Prod", modelID)
	ch2 := seedChannelForPoliciesTest(t, a, "OpenAI Staging", modelID)

	body := map[string]interface{}{
		"name":                  "My Route Policy",
		"user_level":            "premium",
		"model_id":              modelID.String(),
		"priority":              100,
		"candidate_channel_ids": []string{ch1.String(), ch2.String()},
		"fallback_policy":       "next_policy",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data policyResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.ID == "" {
		t.Fatal("id is empty")
	}
	if resp.Data.Name != "My Route Policy" {
		t.Errorf("name = %s, want 'My Route Policy'", resp.Data.Name)
	}
	if resp.Data.UserLevel != "premium" {
		t.Errorf("user_level = %s, want 'premium'", resp.Data.UserLevel)
	}
	if resp.Data.ModelID == nil || *resp.Data.ModelID != modelID.String() {
		t.Errorf("model_id = %v, want %s", resp.Data.ModelID, modelID.String())
	}
	if resp.Data.Priority != 100 {
		t.Errorf("priority = %d, want 100", resp.Data.Priority)
	}
	if resp.Data.FallbackPolicy != "next_policy" {
		t.Errorf("fallback_policy = %s, want 'next_policy'", resp.Data.FallbackPolicy)
	}
	if resp.Data.TenantID != nil {
		t.Errorf("tenant_id = %v, want nil", resp.Data.TenantID)
	}
	if !resp.Data.IsActive {
		t.Errorf("is_active = false, want true")
	}
	if len(resp.Data.CandidateChannelIDs) != 2 {
		t.Errorf("candidate_channel_ids length = %d, want 2", len(resp.Data.CandidateChannelIDs))
	}

	// Verify database state
	var count int
	err := a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM route_policies WHERE name = $1`, "My Route Policy",
	).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected 1 policy in DB, got %d (err: %v)", count, err)
	}
}

func TestHandleCreatePolicy_WithNilTenantID(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-niltenant@example.com", "pass", "Admin Nil Tenant")

	body := map[string]interface{}{
		"name":                  "Tenantless Policy",
		"user_level":            "free",
		"priority":              5,
		"candidate_channel_ids": []string{},
		"fallback_policy":       "disabled",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data policyResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.TenantID != nil {
		t.Errorf("tenant_id = %v, want nil", resp.Data.TenantID)
	}
}

func TestHandleCreatePolicy_WithNonNilTenantID(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-tenant@example.com", "pass", "Admin Tenant")

	tenantID := uuid.New().String()

	body := map[string]interface{}{
		"name":                  "Tenanted Policy",
		"tenant_id":             tenantID,
		"user_level":            "enterprise",
		"priority":              50,
		"candidate_channel_ids": []string{},
		"fallback_policy":       "tenant_default",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data policyResponse `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.TenantID == nil || *resp.Data.TenantID != tenantID {
		t.Errorf("tenant_id = %v, want %s", resp.Data.TenantID, tenantID)
	}
}

func TestHandleCreatePolicy_InvalidTenantID(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-badtenant@example.com", "pass", "Admin Bad Tenant")

	body := map[string]interface{}{
		"name":                  "Bad Tenant Policy",
		"tenant_id":             "not-a-uuid",
		"user_level":            "basic",
		"priority":              10,
		"candidate_channel_ids": []string{},
		"fallback_policy":       "disabled",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleCreatePolicy_InvalidModelID(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-badmodel@example.com", "pass", "Admin Bad Model")

	body := map[string]interface{}{
		"name":                  "Bad Model Policy",
		"model_id":              "not-a-uuid",
		"user_level":            "basic",
		"priority":              10,
		"candidate_channel_ids": []string{},
		"fallback_policy":       "disabled",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleCreatePolicy_ValidFallbackPolicies(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-create-fallback@example.com", "pass", "Admin Fallback")

	tests := []struct {
		name           string
		fallbackPolicy string
	}{
		{"disabled", "disabled"},
		{"tenant_default", "tenant_default"},
		{"shared_allowed", "shared_allowed"},
		{"next_policy", "next_policy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]interface{}{
				"name":                  "Fallback Test " + tt.name,
				"user_level":            "basic",
				"priority":              10,
				"candidate_channel_ids": []string{},
				"fallback_policy":       tt.fallbackPolicy,
			}
			bodyBytes, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/admin/policies", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setAdminCtx(req, user.ID.String())
			w := httptest.NewRecorder()

			handler := HandleCreatePolicy(a)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d for fallback %q, body: %s", w.Code, http.StatusCreated, tt.fallbackPolicy, w.Body.String())
			}
		})
	}
}

// =============================================================================
// HandleUpdatePolicy Tests
// =============================================================================

func TestHandleUpdatePolicy_NoAuth(t *testing.T) {
	a := appForPoliciesTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUpdatePolicy_NotAdmin(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-upd-notadmin@example.com", "pass", "Not Admin")

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/some-id", nil)
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleUpdatePolicy_NotFound(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-upd-404@example.com", "pass", "Admin Upd 404")

	nonExistentID := uuid.New().String()
	body := map[string]string{"name": "Updated"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/"+nonExistentID, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleUpdatePolicy_InvalidID(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-upd-badid@example.com", "pass", "Admin Upd BadID")

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/not-a-uuid", nil)
	req = chiRouteCtx(req, "id", "not-a-uuid")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdatePolicy_InvalidBody(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-upd-badbody@example.com", "pass", "Admin Upd Body")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	chID := seedChannelForPoliciesTest(t, a, "Test Channel", modelID)
	policyID := seedPolicyForTest(t, a, "Original Policy", &modelID, []uuid.UUID{chID})

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/"+policyID.String(), bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", policyID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdatePolicy_InvalidFallbackPolicy(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-upd-badfp@example.com", "pass", "Admin Upd Fallback")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	chID := seedChannelForPoliciesTest(t, a, "Test Channel", modelID)
	policyID := seedPolicyForTest(t, a, "Original Policy", &modelID, []uuid.UUID{chID})

	body := map[string]interface{}{"fallback_policy": "invalid_value"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/"+policyID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", policyID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleUpdatePolicy_Success(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-upd@example.com", "pass", "Admin Update")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	ch1 := seedChannelForPoliciesTest(t, a, "OpenAI Prod", modelID)
	ch2 := seedChannelForPoliciesTest(t, a, "OpenAI Staging", modelID)
	policyID := seedPolicyForTest(t, a, "Original Policy", &modelID, []uuid.UUID{ch1})

	body := map[string]interface{}{
		"name":                  "Updated Policy",
		"user_level":            "enterprise",
		"priority":              200,
		"candidate_channel_ids": []string{ch2.String()},
		"fallback_policy":       "tenant_default",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/"+policyID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", policyID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data policyResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.Name != "Updated Policy" {
		t.Errorf("name = %s, want 'Updated Policy'", resp.Data.Name)
	}
	if resp.Data.Priority != 200 {
		t.Errorf("priority = %d, want 200", resp.Data.Priority)
	}
	if resp.Data.FallbackPolicy != "tenant_default" {
		t.Errorf("fallback_policy = %s, want 'tenant_default'", resp.Data.FallbackPolicy)
	}

	// Verify DB state
	var name string
	var priority int
	err := a.Pool.QueryRow(context.Background(),
		`SELECT name, priority FROM route_policies WHERE id = $1`, policyID,
	).Scan(&name, &priority)
	if err != nil {
		t.Fatalf("query policy: %v", err)
	}
	if name != "Updated Policy" {
		t.Errorf("db name = %s, want 'Updated Policy'", name)
	}
	if priority != 200 {
		t.Errorf("db priority = %d, want 200", priority)
	}
}

func TestHandleUpdatePolicy_ClearTenantID(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-upd-cleartenant@example.com", "pass", "Admin Clear Tenant")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	chID := seedChannelForPoliciesTest(t, a, "Test Channel", modelID)

	// Create a policy WITH tenant_id set
	tenantID := uuid.New()
	policyID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO route_policies (id, name, tenant_id, user_level, model_id, priority, candidate_channel_ids, fallback_policy, is_active, created_at, updated_at)
		 VALUES ($1, $2, $3, 'basic', $4, 10, $5, 'disabled', true, $6, $6)`,
		policyID, "Tenant Policy", tenantID, modelID, []uuid.UUID{chID}, now,
	)
	if err != nil {
		t.Fatalf("seed policy: %v", err)
	}

	// Clear tenant_id
	body := map[string]interface{}{"tenant_id": nil}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/policies/"+policyID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", policyID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdatePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify tenant_id is now nil in DB
	var dbTenantID *uuid.UUID
	err = a.Pool.QueryRow(context.Background(),
		`SELECT tenant_id FROM route_policies WHERE id = $1`, policyID,
	).Scan(&dbTenantID)
	if err != nil {
		t.Fatalf("query policy: %v", err)
	}
	if dbTenantID != nil {
		t.Errorf("tenant_id = %v, want nil", dbTenantID)
	}
}

// =============================================================================
// HandleDeletePolicy Tests
// =============================================================================

func TestHandleDeletePolicy_NoAuth(t *testing.T) {
	a := appForPoliciesTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/policies/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleDeletePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeletePolicy_NotAdmin(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-del-notadmin@example.com", "pass", "Not Admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/policies/some-id", nil)
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeletePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleDeletePolicy_NotFound(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-del-404@example.com", "pass", "Admin Del 404")

	nonExistentID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/policies/"+nonExistentID, nil)
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeletePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleDeletePolicy_InvalidID(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-del-badid@example.com", "pass", "Admin Del BadID")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/policies/not-a-uuid", nil)
	req = chiRouteCtx(req, "id", "not-a-uuid")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeletePolicy(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDeletePolicy_Success(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-del@example.com", "pass", "Admin Delete")
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	chID := seedChannelForPoliciesTest(t, a, "Test Channel", modelID)
	policyID := seedPolicyForTest(t, a, "To Be Deleted", &modelID, []uuid.UUID{chID})

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/policies/"+policyID.String(), nil)
	req = chiRouteCtx(req, "id", policyID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeletePolicy(a)
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

	// Verify soft-delete (is_active = false)
	var isActive bool
	err := a.Pool.QueryRow(context.Background(),
		`SELECT is_active FROM route_policies WHERE id = $1`, policyID,
	).Scan(&isActive)
	if err != nil {
		t.Fatalf("query policy: %v", err)
	}
	if isActive {
		t.Errorf("is_active should be false after soft-delete")
	}
}

func TestHandleDeletePolicy_AlreadyInactive(t *testing.T) {
	a := appForPoliciesTest(t)
	user := seedUserForPoliciesTest(t, a, "pol-del-inactive@example.com", "pass", "Admin Del Inactive")

	// Create an already-inactive policy
	policyID := uuid.New()
	now := time.Now().UTC()
	modelID := seedModelForPoliciesTest(t, a, "gpt-4o", "openai")
	chID := seedChannelForPoliciesTest(t, a, "Test Channel", modelID)

	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO route_policies (id, name, user_level, model_id, priority, candidate_channel_ids, fallback_policy, is_active, created_at, updated_at)
		 VALUES ($1, 'Inactive Policy', 'basic', $2, 10, $3, 'disabled', false, $4, $4)`,
		policyID, modelID, []uuid.UUID{chID}, now,
	)
	if err != nil {
		t.Fatalf("seed policy: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/policies/"+policyID.String(), nil)
	req = chiRouteCtx(req, "id", policyID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeletePolicy(a)
	handler.ServeHTTP(w, req)

	// Should still return 200 (idempotent)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

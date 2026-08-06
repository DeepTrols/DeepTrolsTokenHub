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
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// appForTenantsTest creates a minimal App with pool, config, and repos wired.
func appForTenantsTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-tenants-32-bytes",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Tenants: tenant.NewPostgresRepository(pool),
		Users:   user.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedUserForTenantsTest creates a user with bcrypt hash.
func seedUserForTenantsTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
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
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("seedUserForTenantsTest: create: %v", err)
	}
	return u
}

// seedTenantForTest inserts a tenant and optional domain, returns the tenant.
func seedTenantForTest(t *testing.T, a *app.App, code, name string, status domain.TenantStatus) *domain.Tenant {
	t.Helper()
	now := time.Now().UTC()
	tn := &domain.Tenant{
		ID:        uuid.New(),
		Code:      code,
		Name:      name,
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.Tenants.Create(context.Background(), tn); err != nil {
		t.Fatalf("seedTenantForTest: %v", err)
	}
	return tn
}

// seedTenantDomainForTest inserts a domain for the given tenant.
func seedTenantDomainForTest(t *testing.T, a *app.App, tenantID uuid.UUID, domainName string, isPrimary bool) uuid.UUID {
	t.Helper()
	domainID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO tenant_domains (id, tenant_id, domain, is_primary, created_at) VALUES ($1, $2, $3, $4, $5)`,
		domainID, tenantID, domainName, isPrimary, now,
	)
	if err != nil {
		t.Fatalf("seedTenantDomainForTest: %v", err)
	}
	return domainID
}

// setAdminCtxForTenants adds user_id and role="admin" to the request context.
func setAdminCtxForTenants(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxRoleKey, "admin")
	return r.WithContext(ctx)
}

// setNonAdminCtxForTenants adds user_id without admin role.
func setNonAdminCtxForTenants(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxRoleKey, "user")
	return r.WithContext(ctx)
}

// =============================================================================
// HandleListTenants Tests
// =============================================================================

func TestHandleListTenants_NoAuth(t *testing.T) {
	a := appForTenantsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil)
	w := httptest.NewRecorder()

	handler := HandleListTenants(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListTenants_NotAdmin(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "notadmin@tens.com", "pass", "Not Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil)
	req = setNonAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTenants(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleListTenants_EmptyList(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-empty@tens.com", "pass", "Admin Empty")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil)
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTenants(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].([]interface{})
	if data == nil || len(data) != 0 {
		t.Errorf("expected empty data array, got %v", data)
	}
}

func TestHandleListTenants_ReturnsTenants(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-list@tens.com", "pass", "Admin List")

	seedTenantForTest(t, a, "tenant-a", "Tenant A", domain.TenantStatusActive)
	seedTenantForTest(t, a, "tenant-b", "Tenant B", domain.TenantStatusPendingReview)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil)
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTenants(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			ID           string  `json:"id"`
			Code         string  `json:"code"`
			Name         string  `json:"name"`
			Status       string  `json:"status"`
			OwnerID      *string `json:"owner_id"`
			StatusReason string  `json:"status_reason"`
			CreatedAt    string  `json:"created_at"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 tenants, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
}

// =============================================================================
// HandleGetTenant Tests
// =============================================================================

func TestHandleGetTenant_NoAuth(t *testing.T) {
	a := appForTenantsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleGetTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetTenant_NotFound(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-get-nf@tens.com", "pass", "Admin Get NF")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+uuid.New().String(), nil)
	req = chiRouteCtx(req, "id", uuid.New().String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleGetTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleGetTenant_WithDomains(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-get@tens.com", "pass", "Admin Get")

	tn := seedTenantForTest(t, a, "tenant-with-dom", "Tenant With Domains", domain.TenantStatusActive)
	seedTenantDomainForTest(t, a, tn.ID, "api.example.com", true)
	seedTenantDomainForTest(t, a, tn.ID, "staging.example.com", false)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+tn.ID.String(), nil)
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleGetTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID      string `json:"id"`
			Code    string `json:"code"`
			Name    string `json:"name"`
			Status  string `json:"status"`
			Domains []struct {
				ID        uuid.UUID `json:"id"`
				Domain    string    `json:"domain"`
				IsPrimary bool      `json:"is_primary"`
			} `json:"domains"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.ID != tn.ID.String() {
		t.Errorf("id = %s, want %s", resp.Data.ID, tn.ID.String())
	}
	if len(resp.Data.Domains) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(resp.Data.Domains))
	}
}

// =============================================================================
// HandleCreateTenant Tests
// =============================================================================

func TestHandleCreateTenant_NoAuth(t *testing.T) {
	a := appForTenantsTest(t)

	body := map[string]string{"code": "new-tenant", "name": "New Tenant"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleCreateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateTenant_InvalidBody(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-badbody@tens.com", "pass", "Admin BadBody")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateTenant_MissingCode(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-nocode@tens.com", "pass", "Admin NoCode")

	body := map[string]string{"name": "No Code"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateTenant_Success(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-create@tens.com", "pass", "Admin Create")

	body := map[string]interface{}{
		"code": "my-tenant",
		"name": "My Tenant",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID     string `json:"id"`
			Code   string `json:"code"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Code != "my-tenant" {
		t.Errorf("code = %s, want 'my-tenant'", resp.Data.Code)
	}
	if resp.Data.Name != "My Tenant" {
		t.Errorf("name = %s, want 'My Tenant'", resp.Data.Name)
	}
	if resp.Data.Status != string(domain.TenantStatusPendingReview) {
		t.Errorf("status = %s, want %s", resp.Data.Status, domain.TenantStatusPendingReview)
	}
}

func TestHandleCreateTenant_DuplicateCode(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-dup@tens.com", "pass", "Admin Dup")

	seedTenantForTest(t, a, "dup-tenant", "Existing Tenant", domain.TenantStatusActive)

	body := map[string]interface{}{
		"code": "dup-tenant",
		"name": "Duplicate Attempt",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

// =============================================================================
// HandleUpdateTenant Tests
// =============================================================================

func TestHandleUpdateTenant_NotFound(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-upd-nf@tens.com", "pass", "Admin Upd NF")

	body := map[string]string{"name": "Updated"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+uuid.New().String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", uuid.New().String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateTenant_Success(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-upd@tens.com", "pass", "Admin Upd")

	tn := seedTenantForTest(t, a, "updatable-tenant", "Old Name", domain.TenantStatusActive)

	body := map[string]interface{}{
		"name":          "New Name",
		"status_reason": "renamed for clarity",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify in DB
	updated, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if updated.Name != "New Name" {
		t.Errorf("name = %s, want 'New Name'", updated.Name)
	}
	if updated.StatusReason != "renamed for clarity" {
		t.Errorf("status_reason = %s, want 'renamed for clarity'", updated.StatusReason)
	}
}

func TestHandleUpdateTenant_ValidStatusTransition(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-transok@tens.com", "pass", "Admin TransOK")

	tn := seedTenantForTest(t, a, "trans-ok", "Transition OK", domain.TenantStatusPendingReview)

	body := map[string]interface{}{
		"status":        string(domain.TenantStatusActive),
		"status_reason": "approved after review",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	updated, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if updated.Status != domain.TenantStatusActive {
		t.Errorf("status = %s, want %s", updated.Status, domain.TenantStatusActive)
	}
}

func TestHandleUpdateTenant_InvalidStatusTransition(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-transbad@tens.com", "pass", "Admin TransBad")

	tn := seedTenantForTest(t, a, "trans-bad", "Transition Bad", domain.TenantStatusActive)

	body := map[string]interface{}{
		"status":        string(domain.TenantStatusPendingReview),
		"status_reason": "trying to go backwards",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleUpdateTenant_InvalidStatusValue(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-badval@tens.com", "pass", "Admin BadVal")

	tn := seedTenantForTest(t, a, "bad-val", "Bad Value", domain.TenantStatusPendingReview)

	body := map[string]interface{}{
		"status": "nonexistent_status",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateTenant_NotAdmin(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "user-upd@tens.com", "pass", "User Upd")

	tn := seedTenantForTest(t, a, "user-upd-tenant", "User Update Attempt", domain.TenantStatusActive)

	body := map[string]string{"name": "Hacked"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setNonAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// =============================================================================
// HandleDeleteTenant (Terminate) Tests
// =============================================================================

func TestHandleDeleteTenant_NoAuth(t *testing.T) {
	a := appForTenantsTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleDeleteTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteTenant_NotFound(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-del-nf@tens.com", "pass", "Admin Del NF")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+uuid.New().String(), nil)
	req = chiRouteCtx(req, "id", uuid.New().String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteTenant_Success(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-del@tens.com", "pass", "Admin Del")

	tn := seedTenantForTest(t, a, "terminate-me", "Terminate Me", domain.TenantStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String(), nil)
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify terminated in DB
	terminated, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if terminated.Status != domain.TenantStatusTerminated {
		t.Errorf("status = %s, want %s", terminated.Status, domain.TenantStatusTerminated)
	}
	if terminated.StatusReason != "admin action" {
		t.Errorf("status_reason = %s, want 'admin action'", terminated.StatusReason)
	}
}

// =============================================================================
// HandleAddTenantDomain Tests
// =============================================================================

func TestHandleAddTenantDomain_NoAuth(t *testing.T) {
	a := appForTenantsTest(t)

	body := map[string]interface{}{"domain": "api.example.com", "is_primary": true}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/some-id/domains", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleAddTenantDomain(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleAddTenantDomain_TenantNotFound(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-dom-nf@tens.com", "pass", "Admin Dom NF")

	body := map[string]interface{}{"domain": "api.example.com"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/"+uuid.New().String()+"/domains", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", uuid.New().String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddTenantDomain(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAddTenantDomain_MissingDomain(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-dom-miss@tens.com", "pass", "Admin Dom Miss")

	tn := seedTenantForTest(t, a, "dom-miss", "Domain Missing", domain.TenantStatusActive)

	body := map[string]interface{}{"is_primary": true}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/"+tn.ID.String()+"/domains", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddTenantDomain(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddTenantDomain_Success(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-dom@tens.com", "pass", "Admin Dom")

	tn := seedTenantForTest(t, a, "add-dom-tenant", "Add Domain Tenant", domain.TenantStatusActive)

	body := map[string]interface{}{
		"domain":     "my.example.com",
		"is_primary": true,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/"+tn.ID.String()+"/domains", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddTenantDomain(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID        string `json:"id"`
			Domain    string `json:"domain"`
			IsPrimary bool   `json:"is_primary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Domain != "my.example.com" {
		t.Errorf("domain = %s, want 'my.example.com'", resp.Data.Domain)
	}
	if !resp.Data.IsPrimary {
		t.Error("is_primary should be true")
	}

	// Verify in DB
	var count int
	err := a.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM tenant_domains WHERE tenant_id = $1 AND domain = $2", tn.ID, "my.example.com").Scan(&count)
	if err != nil {
		t.Fatalf("query domain count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 domain row, got %d", count)
	}
}

// =============================================================================
// HandleRemoveTenantDomain Tests
// =============================================================================

func TestHandleRemoveTenantDomain_TenantNotFound(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-rmdom-nf@tens.com", "pass", "Admin RmDom NF")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+uuid.New().String()+"/domains/"+uuid.New().String(), nil)
	req = chiRouteMultiCtx(req, map[string]string{"id": uuid.New().String(), "domainId": uuid.New().String()})
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleRemoveTenantDomain(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleRemoveTenantDomain_Success(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-rmdom@tens.com", "pass", "Admin RmDom")

	tn := seedTenantForTest(t, a, "rm-dom-tenant", "Remove Domain Tenant", domain.TenantStatusActive)
	domainID := seedTenantDomainForTest(t, a, tn.ID, "remove-me.example.com", false)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String()+"/domains/"+domainID.String(), nil)
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "domainId": domainID.String()})
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleRemoveTenantDomain(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify deleted from DB
	var count int
	err := a.Pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM tenant_domains WHERE id = $1", domainID).Scan(&count)
	if err != nil {
		t.Fatalf("query domain: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 domain rows after delete, got %d", count)
	}
}

func TestHandleRemoveTenantDomain_NotFound(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-rmdom-ns@tens.com", "pass", "Admin RmDom NS")

	tn := seedTenantForTest(t, a, "rm-dom-ns", "Domain Not Found", domain.TenantStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String()+"/domains/"+uuid.New().String(), nil)
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "domainId": uuid.New().String()})
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleRemoveTenantDomain(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

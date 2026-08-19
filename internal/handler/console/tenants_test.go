package console

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/jackc/pgx/v5"
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

// seedMembershipForTenantsTest inserts a membership row for the given user+tenant.
func seedMembershipForTenantsTest(t *testing.T, a *app.App, tenantID, userID uuid.UUID, role domain.MembershipRole, status domain.MembershipStatus) {
	t.Helper()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status, joined_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		uuid.New(), tenantID, userID, string(role), string(status), now, now, now,
	)
	if err != nil {
		t.Fatalf("seedMembershipForTenantsTest: %v", err)
	}
}

// seedMembershipForAdminTest inserts a membership via the repository and
// returns it. Shared by team/enterprise/ledger tests.
func seedMembershipForAdminTest(t *testing.T, a *app.App, tenantID, userID uuid.UUID, role domain.MembershipRole, status domain.MembershipStatus) *domain.TenantMembership {
	t.Helper()
	m := &domain.TenantMembership{
		ID:       uuid.New(),
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
		Status:   status,
		JoinedAt: time.Now().UTC(),
	}
	if err := a.Memberships.Create(context.Background(), m); err != nil {
		t.Fatalf("seedMembershipForAdminTest: %v", err)
	}
	return m
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

func TestHandleDeleteTenant_HardDelete(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-del@tens.com", "pass", "Admin Del")

	tn := seedTenantForTest(t, a, "delete-me", "Delete Me", domain.TenantStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String(), nil)
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify the tenant row is permanently gone (hard delete, not a status flip).
	if _, err := a.Tenants.FindByID(context.Background(), tn.ID); err == nil {
		t.Error("expected tenant to be deleted, but FindByID succeeded")
	}
}

func TestHandleDeleteTenant_PlatformTenant_Forbidden(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-del-plat@tens.com", "pass", "Admin Del Plat")

	tn := seedTenantForTest(t, a, app.PlatformTenantCode, "DeepTrols 平台企业", domain.TenantStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String(), nil)
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	// The platform tenant must remain active.
	kept, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if kept.Status != domain.TenantStatusActive {
		t.Errorf("status = %s, want %s (platform tenant must stay active)", kept.Status, domain.TenantStatusActive)
	}
}

func TestHandleDeleteTenant_CascadeCleanup(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-cascade@tens.com", "pass", "Admin Cascade")
	tn := seedTenantForTest(t, a, "cascade-me", "Cascade Me", domain.TenantStatusActive)
	ctx := context.Background()

	// Seed dependent rows that must be removed together with the tenant.
	seedMembershipForTenantsTest(t, a, tn.ID, user.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	modelID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category) VALUES ($1, 'cascade-model', 'test', 'chat')`,
		modelID); err != nil {
		t.Fatalf("insert model: %v", err)
	}
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO tenant_models (id, tenant_id, model_id) VALUES ($1, $2, $3)`,
		uuid.New(), tn.ID, modelID); err != nil {
		t.Fatalf("insert tenant_model: %v", err)
	}

	poolID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO quota_pools (id, tenant_id, dimension, unit_name) VALUES ($1, $2, 'token', 'token')`,
		poolID, tn.ID); err != nil {
		t.Fatalf("insert quota pool: %v", err)
	}
	allocID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO quota_allocations (id, pool_id, user_id, allocated_amount) VALUES ($1, $2, $3, 1000)`,
		allocID, poolID, user.ID); err != nil {
		t.Fatalf("insert quota allocation: %v", err)
	}
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO quota_ledger (id, allocation_id, idempotency_key, action, amount, balance_after) VALUES ($1, $2, $3, 'grant', 1000, 1000)`,
		uuid.New(), allocID, uuid.New().String()); err != nil {
		t.Fatalf("insert quota ledger: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String(), nil)
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()
	HandleDeleteTenant(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Tenant row gone.
	if _, err := a.Tenants.FindByID(ctx, tn.ID); err == nil {
		t.Error("expected tenant to be deleted, but FindByID succeeded")
	}

	// All tenant-owned rows gone.
	assertTenantCount := func(query string, want int, desc string) {
		var n int
		if err := a.Pool.QueryRow(ctx, query, tn.ID).Scan(&n); err != nil {
			t.Fatalf("%s: %v", desc, err)
		}
		if n != want {
			t.Errorf("%s: got %d rows, want %d", desc, n, want)
		}
	}
	assertTenantCount(`SELECT COUNT(*) FROM tenant_memberships WHERE tenant_id = $1`, 0, "tenant_memberships")
	assertTenantCount(`SELECT COUNT(*) FROM tenant_models WHERE tenant_id = $1`, 0, "tenant_models")
	assertTenantCount(`SELECT COUNT(*) FROM quota_pools WHERE tenant_id = $1`, 0, "quota_pools")

	var allocCount int
	if err := a.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM quota_allocations WHERE pool_id = $1`, poolID).Scan(&allocCount); err != nil {
		t.Fatalf("quota_allocations count: %v", err)
	}
	if allocCount != 0 {
		t.Errorf("quota_allocations: got %d rows, want 0", allocCount)
	}

	var ledgerCount int
	if err := a.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM quota_ledger WHERE allocation_id = $1`, allocID).Scan(&ledgerCount); err != nil {
		t.Fatalf("quota_ledger count: %v", err)
	}
	if ledgerCount != 0 {
		t.Errorf("quota_ledger: got %d rows, want 0", ledgerCount)
	}

	// The global model row is NOT tenant-owned and must survive.
	var modelCount int
	if err := a.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM models WHERE id = $1`, modelID).Scan(&modelCount); err != nil {
		t.Fatalf("models count: %v", err)
	}
	if modelCount != 1 {
		t.Errorf("models: got %d rows, want 1 (global model must not be deleted)", modelCount)
	}
}

func TestHandleUpdateTenant_PlatformTenant_StatusChange_Forbidden(t *testing.T) {
	a := appForTenantsTest(t)
	user := seedUserForTenantsTest(t, a, "admin-upd-plat@tens.com", "pass", "Admin Upd Plat")

	tn := seedTenantForTest(t, a, app.PlatformTenantCode, "DeepTrols 平台企业", domain.TenantStatusActive)

	body := map[string]interface{}{
		"status": string(domain.TenantStatusSuspended),
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateTenant(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	kept, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if kept.Status != domain.TenantStatusActive {
		t.Errorf("status = %s, want %s (platform tenant must stay active)", kept.Status, domain.TenantStatusActive)
	}
}

// =============================================================================
// HandleCreateTenant — Owner Provisioning Tests
// =============================================================================

// postCreateTenant issues a POST to HandleCreateTenant with the given body and
// admin context, returning the recorder.
func postCreateTenant(t *testing.T, a *app.App, admin *domain.User, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()
	HandleCreateTenant(a).ServeHTTP(w, req)
	return w
}

func TestHandleCreateTenant_WithOwnerEmail_CreatesOwner(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-owner@tens.com", "pass", "Admin Owner")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":           "acme",
		"name":           "Acme Inc",
		"owner_email":    "ceo@acme.com",
		"owner_password": "acme-owner-pass-1",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID      string  `json:"id"`
			OwnerID *string `json:"owner_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.OwnerID == nil || *resp.Data.OwnerID == "" {
		t.Fatalf("owner_id missing in response: %s", w.Body.String())
	}

	ctx := context.Background()

	// Owner user created as an active enterprise account.
	var userID, userType string
	var userStatus domain.UserStatus
	if err := a.Pool.QueryRow(ctx,
		`SELECT id, user_type, status FROM users WHERE email = $1`, "ceo@acme.com",
	).Scan(&userID, &userType, &userStatus); err != nil {
		t.Fatalf("owner user not found: %v", err)
	}
	if userType != string(domain.UserTypeEnterprise) {
		t.Errorf("user_type = %s, want %s", userType, domain.UserTypeEnterprise)
	}
	if userStatus != domain.UserStatusActive {
		t.Errorf("status = %s, want %s", userStatus, domain.UserStatusActive)
	}

	// Tenant owner_id points at the created user.
	var tenantOwnerID *string
	if err := a.Pool.QueryRow(ctx,
		`SELECT owner_id FROM tenants WHERE code = 'acme'`,
	).Scan(&tenantOwnerID); err != nil {
		t.Fatalf("tenant lookup: %v", err)
	}
	if tenantOwnerID == nil || *tenantOwnerID != userID {
		t.Errorf("tenant owner_id = %v, want %s", tenantOwnerID, userID)
	}

	// Active owner membership links user and tenant.
	var memRole, memStatus string
	if err := a.Pool.QueryRow(ctx,
		`SELECT role, status FROM tenant_memberships WHERE user_id = $1 AND tenant_id = $2`,
		userID, resp.Data.ID,
	).Scan(&memRole, &memStatus); err != nil {
		t.Fatalf("owner membership not found: %v", err)
	}
	if memRole != string(domain.MembershipRoleOwner) || memStatus != string(domain.MembershipStatusActive) {
		t.Errorf("membership role/status = %s/%s, want owner/active", memRole, memStatus)
	}

	// A zero-balance wallet exists for the owner account.
	var walletUserID string
	if err := a.Pool.QueryRow(ctx,
		`SELECT user_id FROM wallets WHERE user_id = $1`, userID,
	).Scan(&walletUserID); err != nil {
		t.Fatalf("owner wallet not found: %v", err)
	}
}

func TestHandleCreateTenant_WithOwnerEmail_ExistingUserLinksMembership(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-link@tens.com", "pass", "Admin Link")
	owner := seedUserForTenantsTest(t, a, "existing-owner@acme.com", "owner-password-1", "Existing Owner")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":           "acme-2",
		"name":           "Acme Two",
		"owner_email":    owner.Email,
		"owner_password": "owner-password-1",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	ctx := context.Background()

	// No duplicate user row: the existing account is reused.
	var count int
	if err := a.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE email = $1`, owner.Email,
	).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Errorf("user count = %d, want 1", count)
	}

	// Owner membership links the existing user.
	var memTenantID string
	if err := a.Pool.QueryRow(ctx,
		`SELECT tenant_id FROM tenant_memberships WHERE user_id = $1 AND role = 'owner'`,
		owner.ID,
	).Scan(&memTenantID); err != nil {
		t.Fatalf("owner membership not found: %v", err)
	}
}

func TestHandleCreateTenant_WithOwnerEmail_ExistingUserWrongPassword(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-wrong@tens.com", "pass", "Admin Wrong")
	seedUserForTenantsTest(t, a, "owner-wrong@acme.com", "real-password-1", "Owner Wrong")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":           "acme-3",
		"name":           "Acme Three",
		"owner_email":    "owner-wrong@acme.com",
		"owner_password": "not-the-password",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	// No tenant may be created on a failed owner resolution.
	if _, err := a.Tenants.FindByCode(context.Background(), "acme-3"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("tenant should not exist, err = %v", err)
	}
}

func TestHandleCreateTenant_WithOwnerEmail_MissingPassword(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-nopw@tens.com", "pass", "Admin NoPw")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":        "acme-4",
		"name":        "Acme Four",
		"owner_email": "ceo4@acme.com",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleCreateTenant_WithOwnerEmail_ShortPasswordNewUser(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-short@tens.com", "pass", "Admin Short")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":           "acme-5",
		"name":           "Acme Five",
		"owner_email":    "ceo5@acme.com",
		"owner_password": "short",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleCreateTenant_WithOwnerEmailAndOwnerID_Conflict(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-both@tens.com", "pass", "Admin Both")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":           "acme-6",
		"name":           "Acme Six",
		"owner_email":    "ceo6@acme.com",
		"owner_password": "acme-owner-pass-6",
		"owner_id":       uuid.New().String(),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleCreateTenant_WithOwnerID_InvalidUUID(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-badid@tens.com", "pass", "Admin BadId")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":     "acme-7",
		"name":     "Acme Seven",
		"owner_id": "not-a-uuid",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleCreateTenant_WithOwnerID_UserNotFound(t *testing.T) {
	a := appForTenantsTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-nf@tens.com", "pass", "Admin Nf")

	w := postCreateTenant(t, a, admin, map[string]interface{}{
		"code":     "acme-8",
		"name":     "Acme Eight",
		"owner_id": uuid.New().String(),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

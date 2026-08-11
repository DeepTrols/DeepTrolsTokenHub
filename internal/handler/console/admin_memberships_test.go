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
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// appForAdminMembershipTest creates an App with tenant, user, and membership
// repositories wired for the admin tenant-member management endpoints.
func appForAdminMembershipTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-admin-members-32",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:        pool,
		Config:      cfg,
		Tenants:     tenant.NewPostgresRepository(pool),
		Users:       user.NewPostgresRepository(pool),
		Memberships: membership.NewPostgresRepository(pool),
		Healthy:     true,
	}
}

// seedMembershipForAdminTest inserts a membership via the repository.
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

// seedUserForAdminMembershipTest creates a user with the given email and returns it.
func seedUserForAdminMembershipTest(t *testing.T, a *app.App, email string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	now := time.Now().UTC()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  "Member " + email,
		Role:         "user",
		UserType:     domain.UserTypePersonal,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("seedUserForAdminMembershipTest: create: %v", err)
	}
	return u
}

// =============================================================================
// HandleAdminListTenantMembers Tests
// =============================================================================

func TestHandleAdminListTenantMembers_NoAuth(t *testing.T) {
	a := appForAdminMembershipTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/some-id/members", nil)
	w := httptest.NewRecorder()

	handler := HandleAdminListTenantMembers(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminListTenantMembers_NotAdmin(t *testing.T) {
	a := appForAdminMembershipTest(t)
	user := seedUserForTenantsTest(t, a, "notadmin@mem.test", "pass", "Not Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/some-id/members", nil)
	req = setNonAdminCtxForTenants(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminListTenantMembers(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleAdminListTenantMembers_InvalidTenantID(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-badid@mem.test", "pass", "Admin BadID")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/not-a-uuid/members", nil)
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminListTenantMembers(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAdminListTenantMembers_TenantNotFound(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-nf@mem.test", "pass", "Admin NF")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+uuid.New().String()+"/members", nil)
	req = chiRouteCtx(req, "id", uuid.New().String())
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminListTenantMembers(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAdminListTenantMembers_EmptyList(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-empty@mem.test", "pass", "Admin Empty")
	tn := seedTenantForTest(t, a, "empty-tenant", "Empty Tenant", domain.TenantStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+tn.ID.String()+"/members", nil)
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminListTenantMembers(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []adminMemberItem `json:"data"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 || resp.Total != 0 {
		t.Errorf("expected empty list, got data=%v total=%d", resp.Data, resp.Total)
	}
}

func TestHandleAdminListTenantMembers_ReturnsMembers(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-list@mem.test", "pass", "Admin List")
	tn := seedTenantForTest(t, a, "list-tenant", "List Tenant", domain.TenantStatusActive)

	member := seedUserForAdminMembershipTest(t, a, "member@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	suspended := seedUserForAdminMembershipTest(t, a, "suspended@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, suspended.ID, domain.MembershipRoleAdmin, domain.MembershipStatusSuspended)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/tenants/"+tn.ID.String()+"/members", nil)
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminListTenantMembers(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []adminMemberItem `json:"data"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 members, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	byID := map[string]adminMemberItem{}
	for _, it := range resp.Data {
		byID[it.ID] = it
	}
	mItem, ok := byID[member.ID.String()]
	if !ok {
		t.Fatal("active member missing from response")
	}
	if mItem.Name != "Member member@mem.test" || mItem.Email != "member@mem.test" {
		t.Errorf("active member enrichment wrong: %+v", mItem)
	}
	if mItem.Status != string(domain.MembershipStatusActive) {
		t.Errorf("active status = %s, want active", mItem.Status)
	}
	sItem, ok := byID[suspended.ID.String()]
	if !ok {
		t.Fatal("suspended member missing from response")
	}
	if sItem.Status != string(domain.MembershipStatusSuspended) {
		t.Errorf("suspended status = %s, want suspended", sItem.Status)
	}
	if sItem.Role != string(domain.MembershipRoleAdmin) {
		t.Errorf("suspended role = %s, want admin", sItem.Role)
	}
}

// =============================================================================
// HandleAdminAddTenantMember Tests
// =============================================================================

func TestHandleAdminAddTenantMember_NoAuth(t *testing.T) {
	a := appForAdminMembershipTest(t)

	body, _ := json.Marshal(map[string]string{"email": "new@mem.test", "role": "member"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/some-id/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleAdminAddTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminAddTenantMember_NotAdmin(t *testing.T) {
	a := appForAdminMembershipTest(t)
	nonAdmin := seedUserForTenantsTest(t, a, "nonadmin-add@mem.test", "pass", "Non Admin")

	body, _ := json.Marshal(map[string]string{"email": "new@mem.test", "role": "member"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/some-id/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = setNonAdminCtxForTenants(req, nonAdmin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminAddTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleAdminAddTenantMember_InvalidBody(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-badbody-add@mem.test", "pass", "Admin BadBody")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/some-id/members", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminAddTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAdminAddTenantMember_InvalidRole(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-badrole@mem.test", "pass", "Admin BadRole")
	tn := seedTenantForTest(t, a, "badrole-tenant", "BadRole Tenant", domain.TenantStatusActive)

	body, _ := json.Marshal(map[string]string{"email": "new@mem.test", "role": "owner"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/"+tn.ID.String()+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminAddTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAdminAddTenantMember_UserNotFound(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-nouser@mem.test", "pass", "Admin NoUser")
	tn := seedTenantForTest(t, a, "nouser-tenant", "NoUser Tenant", domain.TenantStatusActive)

	body, _ := json.Marshal(map[string]string{"email": "ghost@mem.test", "role": "member"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/"+tn.ID.String()+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminAddTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAdminAddTenantMember_Success(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-add@mem.test", "pass", "Admin Add")
	tn := seedTenantForTest(t, a, "add-tenant", "Add Tenant", domain.TenantStatusActive)
	member := seedUserForAdminMembershipTest(t, a, "addme@mem.test")

	body, _ := json.Marshal(map[string]string{"email": "addme@mem.test", "role": "member"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/"+tn.ID.String()+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminAddTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	m, err := a.Memberships.FindByUserAndTenant(context.Background(), member.ID, tn.ID)
	if err != nil {
		t.Fatalf("FindByUserAndTenant after add: %v", err)
	}
	if m.Role != domain.MembershipRoleMember {
		t.Errorf("role = %s, want member", m.Role)
	}
	if m.Status != domain.MembershipStatusActive {
		t.Errorf("status = %s, want active", m.Status)
	}
}

func TestHandleAdminAddTenantMember_AlreadyMember(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-dup@mem.test", "pass", "Admin Dup")
	tn := seedTenantForTest(t, a, "dup-tenant", "Dup Tenant", domain.TenantStatusActive)
	member := seedUserForAdminMembershipTest(t, a, "dupmem@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body, _ := json.Marshal(map[string]string{"email": "dupmem@mem.test", "role": "member"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/tenants/"+tn.ID.String()+"/members", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", tn.ID.String())
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminAddTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

// =============================================================================
// HandleAdminRemoveTenantMember Tests
// =============================================================================

func TestHandleAdminRemoveTenantMember_NoAuth(t *testing.T) {
	a := appForAdminMembershipTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/some-id/members/some-user", nil)
	w := httptest.NewRecorder()

	handler := HandleAdminRemoveTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminRemoveTenantMember_MemberNotFound(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-rm-nf@mem.test", "pass", "Admin RmNF")
	tn := seedTenantForTest(t, a, "rm-nf-tenant", "RmNF Tenant", domain.TenantStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String()+"/members/"+uuid.New().String(), nil)
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "userId": uuid.New().String()})
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminRemoveTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAdminRemoveTenantMember_CannotRemoveOwner(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-rm-owner@mem.test", "pass", "Admin RmOwner")
	tn := seedTenantForTest(t, a, "rm-owner-tenant", "RmOwner Tenant", domain.TenantStatusActive)
	owner := seedUserForAdminMembershipTest(t, a, "owner@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String()+"/members/"+owner.ID.String(), nil)
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "userId": owner.ID.String()})
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminRemoveTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleAdminRemoveTenantMember_Success(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-rm@mem.test", "pass", "Admin Rm")
	tn := seedTenantForTest(t, a, "rm-tenant", "Rm Tenant", domain.TenantStatusActive)
	member := seedUserForAdminMembershipTest(t, a, "rmme@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/"+tn.ID.String()+"/members/"+member.ID.String(), nil)
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "userId": member.ID.String()})
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminRemoveTenantMember(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if _, err := a.Memberships.FindByUserAndTenant(context.Background(), member.ID, tn.ID); err == nil {
		t.Fatal("expected membership to be removed")
	}
}

// =============================================================================
// HandleAdminChangeTenantMemberRole Tests
// =============================================================================

func TestHandleAdminChangeTenantMemberRole_NoAuth(t *testing.T) {
	a := appForAdminMembershipTest(t)

	body, _ := json.Marshal(map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/some-id/members/some-user/role", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleAdminChangeTenantMemberRole(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleAdminChangeTenantMemberRole_InvalidRole(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-ch-role@mem.test", "pass", "Admin ChRole")
	tn := seedTenantForTest(t, a, "ch-role-tenant", "ChRole Tenant", domain.TenantStatusActive)
	member := seedUserForAdminMembershipTest(t, a, "chrole@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body, _ := json.Marshal(map[string]string{"role": "owner"})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String()+"/members/"+member.ID.String()+"/role", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "userId": member.ID.String()})
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminChangeTenantMemberRole(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAdminChangeTenantMemberRole_MemberNotFound(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-ch-nf@mem.test", "pass", "Admin ChNF")
	tn := seedTenantForTest(t, a, "ch-nf-tenant", "ChNF Tenant", domain.TenantStatusActive)

	body, _ := json.Marshal(map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String()+"/members/"+uuid.New().String()+"/role", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "userId": uuid.New().String()})
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminChangeTenantMemberRole(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAdminChangeTenantMemberRole_CannotChangeOwnerRole(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-ch-owner@mem.test", "pass", "Admin ChOwner")
	tn := seedTenantForTest(t, a, "ch-owner-tenant", "ChOwner Tenant", domain.TenantStatusActive)
	owner := seedUserForAdminMembershipTest(t, a, "chowner@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	body, _ := json.Marshal(map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String()+"/members/"+owner.ID.String()+"/role", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "userId": owner.ID.String()})
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminChangeTenantMemberRole(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleAdminChangeTenantMemberRole_Success(t *testing.T) {
	a := appForAdminMembershipTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-ch@mem.test", "pass", "Admin Ch")
	tn := seedTenantForTest(t, a, "ch-tenant", "Ch Tenant", domain.TenantStatusActive)
	member := seedUserForAdminMembershipTest(t, a, "changeme@mem.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body, _ := json.Marshal(map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPut, "/api/admin/tenants/"+tn.ID.String()+"/members/"+member.ID.String()+"/role", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteMultiCtx(req, map[string]string{"id": tn.ID.String(), "userId": member.ID.String()})
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAdminChangeTenantMemberRole(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	m, err := a.Memberships.FindByUserAndTenant(context.Background(), member.ID, tn.ID)
	if err != nil {
		t.Fatalf("FindByUserAndTenant after change: %v", err)
	}
	if m.Role != domain.MembershipRoleAdmin {
		t.Errorf("role = %s, want admin", m.Role)
	}
}

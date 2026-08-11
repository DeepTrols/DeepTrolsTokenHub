package console

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/invitation"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
)

// appForEnterpriseTest wires the repos needed by the enterprise handlers.
func appForEnterpriseTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	return &app.App{
		Pool: pool,
		Config: &config.Config{
			JWT: config.JWTConfig{
				Secret:      "test-jwt-secret-enterprise-32-byte",
				ExpiryHours: 24,
			},
		},
		Users:       user.NewPostgresRepository(pool),
		Tenants:     tenant.NewPostgresRepository(pool),
		Memberships: membership.NewPostgresRepository(pool),
		Invitations: invitation.NewPostgresRepository(pool),
		Healthy:     true,
	}
}

// setTenantCtxForEnterprise adds the user and tenant identity to the request
// context, mimicking what ConsoleAuth injects. An empty tenantID represents a
// personal user without a tenant.
func setTenantCtxForEnterprise(r *http.Request, userID, tenantID, tenantRole string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxTenantIDKey, tenantID)
	ctx = context.WithValue(ctx, jwtutil.CtxTenantRoleKey, tenantRole)
	return r.WithContext(ctx)
}

// seedEnterpriseUserForEnterpriseTest creates a user of type enterprise.
func seedEnterpriseUserForEnterpriseTest(t *testing.T, a *app.App, email string) *domain.User {
	t.Helper()
	return seedUserForLedgerTest(t, a, email, domain.UserTypeEnterprise)
}

// seedInvitationForEnterpriseTest inserts an invitation with the given status.
func seedInvitationForEnterpriseTest(t *testing.T, a *app.App, tenantID, invitedBy uuid.UUID, email string, status domain.InvitationStatus) *domain.TenantInvitation {
	t.Helper()
	now := time.Now().UTC()
	inv := &domain.TenantInvitation{
		ID:        uuid.New(),
		TenantID:  tenantID,
		InvitedBy: invitedBy,
		Email:     email,
		Role:      domain.MembershipRoleMember,
		Token:     uuid.New().String(),
		Status:    status,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
	}
	if err := a.Invitations.Create(context.Background(), inv); err != nil {
		t.Fatalf("seedInvitationForEnterpriseTest: %v", err)
	}
	return inv
}

// =============================================================================
// GET /api/console/enterprise
// =============================================================================

func TestHandleGetEnterprise_NoAuth(t *testing.T) {
	a := appForEnterpriseTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/console/enterprise", nil)
	w := httptest.NewRecorder()

	HandleGetEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetEnterprise_PersonalUser_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	personal := seedUserForTenantsTest(t, a, "personal-ent@test.com", "pass", "Personal")

	req := httptest.NewRequest(http.MethodGet, "/api/console/enterprise", nil)
	req = setTenantCtxForEnterprise(req, personal.ID.String(), "", "")
	w := httptest.NewRecorder()

	HandleGetEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleGetEnterprise_NonMember_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-get-2", "Enterprise Two", domain.TenantStatusActive)
	outsider := seedEnterpriseUserForEnterpriseTest(t, a, "outsider@test.com")

	// Context claims a tenant, but there is no membership row for this user.
	req := httptest.NewRequest(http.MethodGet, "/api/console/enterprise", nil)
	req = setTenantCtxForEnterprise(req, outsider.ID.String(), tn.ID.String(), "")
	w := httptest.NewRecorder()

	HandleGetEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleGetEnterprise_Member_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-get-ok", "Acme Corp", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner@acme.test")
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member@acme.test")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/console/enterprise", nil)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleGetEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp enterpriseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != tn.ID.String() {
		t.Errorf("id = %s, want %s", resp.ID, tn.ID.String())
	}
	if resp.Code != "ent-get-ok" {
		t.Errorf("code = %s, want ent-get-ok", resp.Code)
	}
	if resp.Name != "Acme Corp" {
		t.Errorf("name = %s, want Acme Corp", resp.Name)
	}
	if resp.MemberCount != 2 {
		t.Errorf("member_count = %d, want 2", resp.MemberCount)
	}
}

// =============================================================================
// PUT /api/console/enterprise
// =============================================================================

func TestHandleUpdateEnterprise_Member_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-upd-1", "Acme", domain.TenantStatusActive)
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-upd@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"name":"Hacked Name"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise", body)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleUpdateEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleUpdateEnterprise_Admin_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-upd-ok", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-upd@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"name":"Acme Renamed","contact_email":"ops@acme.test","contact_phone":"+86-10-1234"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	w := httptest.NewRecorder()

	HandleUpdateEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	updated, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if updated.Name != "Acme Renamed" {
		t.Errorf("name = %s, want Acme Renamed", updated.Name)
	}
	if updated.ContactEmail != "ops@acme.test" {
		t.Errorf("contact_email = %s, want ops@acme.test", updated.ContactEmail)
	}
	if updated.ContactPhone != "+86-10-1234" {
		t.Errorf("contact_phone = %s, want +86-10-1234", updated.ContactPhone)
	}
}

func TestHandleUpdateEnterprise_EmptyName_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-upd-400", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-400@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"name":"   "}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	w := httptest.NewRecorder()

	HandleUpdateEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// PUT /api/console/enterprise/brand
// =============================================================================

func TestHandleUpdateEnterpriseBrand_Admin_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-brand-1", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-brand@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"brand_config":{"logo_url":"https://x/logo.png"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise/brand", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	w := httptest.NewRecorder()

	HandleUpdateEnterpriseBrand(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleUpdateEnterpriseBrand_Owner_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-brand-ok", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-brand@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"brand_config":{"logo_url":"https://x/logo.png","primary_color":"#0a0a0a"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise/brand", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleUpdateEnterpriseBrand(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	updated, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if updated.BrandConfig == nil {
		t.Fatal("brand_config should be persisted, got nil")
	}
	if updated.BrandConfig["primary_color"] != "#0a0a0a" {
		t.Errorf("brand_config.primary_color = %v, want #0a0a0a", updated.BrandConfig["primary_color"])
	}
}

func TestHandleUpdateEnterpriseBrand_MissingConfig_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-brand-400", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-brand-400@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"brand_config":null}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise/brand", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleUpdateEnterpriseBrand(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// GET /api/console/team/invitations
// =============================================================================

func TestHandleListPendingInvitations_Member_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-inv-list-1", "Acme", domain.TenantStatusActive)
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-inv@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/console/team/invitations", nil)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleListPendingInvitations(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleListPendingInvitations_Admin_OnlyPending(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-inv-list-ok", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-inv@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	_ = seedInvitationForEnterpriseTest(t, a, tn.ID, admin.ID, "pending@acme.test", domain.InvitationStatusPending)
	_ = seedInvitationForEnterpriseTest(t, a, tn.ID, admin.ID, "accepted@acme.test", domain.InvitationStatusAccepted)
	_ = seedInvitationForEnterpriseTest(t, a, tn.ID, admin.ID, "expired@acme.test", domain.InvitationStatusExpired)

	req := httptest.NewRequest(http.MethodGet, "/api/console/team/invitations", nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	w := httptest.NewRecorder()

	HandleListPendingInvitations(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Invitations []enterpriseInvitationItem `json:"invitations"`
		Total       int                        `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("total = %d, want 1 (only pending), body: %s", resp.Total, w.Body.String())
	}
	if len(resp.Invitations) != 1 || resp.Invitations[0].Email != "pending@acme.test" {
		t.Errorf("expected only pending@acme.test, got %+v", resp.Invitations)
	}
}

// =============================================================================
// DELETE /api/console/team/invitations/{id}
// =============================================================================

func TestHandleCancelInvitation_Member_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-inv-del-1", "Acme", domain.TenantStatusActive)
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-del@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/team/invitations/some-id", nil)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleCancelInvitation(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleCancelInvitation_Admin_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-inv-del-ok", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-del@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	inv := seedInvitationForEnterpriseTest(t, a, tn.ID, admin.ID, "pending@acme.test", domain.InvitationStatusPending)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/team/invitations/"+inv.ID.String(), nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "id", inv.ID.String())
	w := httptest.NewRecorder()

	HandleCancelInvitation(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify the status changed via the repo's ListByTenantID.
	invs, err := a.Invitations.ListByTenantID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("ListByTenantID: %v", err)
	}
	if len(invs) != 1 || invs[0].Status != domain.InvitationStatusCancelled {
		t.Errorf("expected invitation cancelled, got %+v", invs)
	}
}

func TestHandleCancelInvitation_OtherTenant_404(t *testing.T) {
	a := appForEnterpriseTest(t)
	tnA := seedTenantForTest(t, a, "ent-inv-del-a", "Tenant A", domain.TenantStatusActive)
	tnB := seedTenantForTest(t, a, "ent-inv-del-b", "Tenant B", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-cross@test.com")
	_ = seedMembershipForAdminTest(t, a, tnB.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	// Invitation belongs to Tenant A, not the caller's Tenant B.
	inv := seedInvitationForEnterpriseTest(t, a, tnA.ID, admin.ID, "other@acme.test", domain.InvitationStatusPending)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/team/invitations/"+inv.ID.String(), nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tnB.ID.String(), "admin")
	req = chiRouteCtx(req, "id", inv.ID.String())
	w := httptest.NewRecorder()

	HandleCancelInvitation(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleCancelInvitation_NonPending_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-inv-del-400", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-del-400@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	inv := seedInvitationForEnterpriseTest(t, a, tn.ID, admin.ID, "accepted@acme.test", domain.InvitationStatusAccepted)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/team/invitations/"+inv.ID.String(), nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "id", inv.ID.String())
	w := httptest.NewRecorder()

	HandleCancelInvitation(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// POST /api/console/team/transfer-ownership
// =============================================================================

func TestHandleTransferOwnership_Member_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-tr-1", "Acme", domain.TenantStatusActive)
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-tr@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"target_user_id":"` + uuid.New().String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleTransferOwnership(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleTransferOwnership_Admin_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-tr-2", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-tr@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"target_user_id":"` + uuid.New().String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	w := httptest.NewRecorder()

	HandleTransferOwnership(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleTransferOwnership_Self_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-tr-self", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-self@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"target_user_id":"` + owner.ID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTransferOwnership(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTransferOwnership_TargetNotAdmin_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-tr-target", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-tgt@test.com")
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-tgt@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"target_user_id":"` + member.ID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTransferOwnership(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTransferOwnership_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-tr-ok", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-ok@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-ok@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"target_user_id":"` + target.ID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTransferOwnership(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Former owner must now be admin.
	oldOwnerMembership, err := a.Memberships.FindByUserAndTenant(context.Background(), owner.ID, tn.ID)
	if err != nil {
		t.Fatalf("find old owner: %v", err)
	}
	if oldOwnerMembership.Role != domain.MembershipRoleAdmin {
		t.Errorf("old owner role = %s, want admin", oldOwnerMembership.Role)
	}

	// Target admin must now be owner.
	targetMembership, err := a.Memberships.FindByUserAndTenant(context.Background(), target.ID, tn.ID)
	if err != nil {
		t.Fatalf("find target: %v", err)
	}
	if targetMembership.Role != domain.MembershipRoleOwner {
		t.Errorf("target role = %s, want owner", targetMembership.Role)
	}

	// Tenant owner_id must point at the target.
	updatedTenant, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("find tenant: %v", err)
	}
	if updatedTenant.OwnerID == nil || *updatedTenant.OwnerID != target.ID {
		t.Errorf("tenant owner_id = %v, want %s", updatedTenant.OwnerID, target.ID.String())
	}
}

func TestHandleTransferOwnership_PlatformTenant_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, app.PlatformTenantCode, "DeepTrols 平台企业", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-plat@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-plat@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"target_user_id":"` + target.ID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTransferOwnership(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}

	// The owner must still own the platform tenant.
	m, err := a.Memberships.FindByUserAndTenant(context.Background(), owner.ID, tn.ID)
	if err != nil {
		t.Fatalf("find owner membership: %v", err)
	}
	if m.Role != domain.MembershipRoleOwner {
		t.Errorf("owner role = %s, want owner", m.Role)
	}
}

// =============================================================================
// C-1: suspended members lose enterprise access
// =============================================================================

func TestHandleGetEnterprise_SuspendedMember_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-sus-1", "Acme", domain.TenantStatusActive)
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-sus@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusSuspended)

	req := httptest.NewRequest(http.MethodGet, "/api/console/enterprise", nil)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleGetEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleUpdateEnterpriseBrand_SuspendedOwner_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-sus-2", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-sus@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusSuspended)

	body := bytes.NewBufferString(`{"brand_config":{"logo_url":"https://x/logo.png"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise/brand", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleUpdateEnterpriseBrand(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleTransferOwnership_SuspendedOwner_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-sus-3", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-sus3@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-sus3@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusSuspended)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"target_user_id":"` + target.ID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTransferOwnership(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// =============================================================================
// H-1: settlement data is hidden from non-admin members
// =============================================================================

// seedTenantWithSettlementForTest seeds a tenant that has settlement fields set.
func seedTenantWithSettlementForTest(t *testing.T, a *app.App, code, name string) *domain.Tenant {
	t.Helper()
	tn := seedTenantForTest(t, a, code, name, domain.TenantStatusActive)
	tn.CreditCode = "91110000TEST"
	tn.BusinessLicense = "BL-TEST-0001"
	tn.SettlementConfig = map[string]any{"bank": "ICBC", "account": "1234"}
	if err := a.Tenants.Update(context.Background(), tn); err != nil {
		t.Fatalf("seedTenantWithSettlementForTest: %v", err)
	}
	return tn
}

func TestHandleGetEnterprise_Member_HidesSettlement(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantWithSettlementForTest(t, a, "ent-hide", "Acme")
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-hide@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/console/enterprise", nil)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleGetEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var raw map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"credit_code", "business_license", "settlement_config"} {
		if _, ok := raw[key]; ok {
			t.Errorf("member response must not expose %q, body: %s", key, w.Body.String())
		}
	}
}

func TestHandleGetEnterprise_Admin_SeesSettlement(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantWithSettlementForTest(t, a, "ent-see", "Acme")
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-see@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/console/enterprise", nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	w := httptest.NewRecorder()

	HandleGetEnterprise(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp enterpriseResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.CreditCode != "91110000TEST" {
		t.Errorf("credit_code = %q, want 91110000TEST", resp.CreditCode)
	}
	if resp.BusinessLicense != "BL-TEST-0001" {
		t.Errorf("business_license = %q, want BL-TEST-0001", resp.BusinessLicense)
	}
	if resp.SettlementConfig == nil || resp.SettlementConfig["bank"] != "ICBC" {
		t.Errorf("settlement_config = %v, want bank=ICBC", resp.SettlementConfig)
	}
}

// =============================================================================
// H-2: brand_config size limit
// =============================================================================

func TestHandleUpdateEnterpriseBrand_TooManyKeys_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-brand-51", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-51@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	// 51 keys exceeds the 50-key cap.
	cfg := make(map[string]any)
	for i := 0; i < 51; i++ {
		cfg[fmt.Sprintf("k%d", i)] = i
	}
	raw, err := json.Marshal(map[string]any{"brand_config": cfg})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/console/enterprise/brand", bytes.NewBuffer(raw))
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleUpdateEnterpriseBrand(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// =============================================================================
// C-2: repeated ownership transfers keep exactly one owner
// =============================================================================

func TestHandleTransferOwnership_OldOwnerCannotTransferAgain_403(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-tr-repeat", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-rep@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-rep@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	// First transfer succeeds: owner -> target.
	body := bytes.NewBufferString(`{"target_user_id":"` + target.ID.String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()
	HandleTransferOwnership(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first transfer status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// The old owner is now an admin; a second transfer from them must be 403.
	body2 := bytes.NewBufferString(`{"target_user_id":"` + owner.ID.String() + `"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body2)
	req2 = setTenantCtxForEnterprise(req2, owner.ID.String(), tn.ID.String(), "owner")
	w2 := httptest.NewRecorder()
	HandleTransferOwnership(a).ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("second transfer status = %d, want %d, body: %s", w2.Code, http.StatusForbidden, w2.Body.String())
	}
}

func TestHandleTransferOwnership_ChainedTransfers_ExactlyOneOwner(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "ent-tr-chain", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-chain@test.com")
	adminB := seedEnterpriseUserForEnterpriseTest(t, a, "admin-b-chain@test.com")
	adminC := seedEnterpriseUserForEnterpriseTest(t, a, "admin-c-chain@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminB.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminC.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	transfer := func(from, to uuid.UUID, role string) int {
		body := bytes.NewBufferString(`{"target_user_id":"` + to.String() + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/console/team/transfer-ownership", body)
		req = setTenantCtxForEnterprise(req, from.String(), tn.ID.String(), role)
		w := httptest.NewRecorder()
		HandleTransferOwnership(a).ServeHTTP(w, req)
		return w.Code
	}

	if code := transfer(owner.ID, adminB.ID, "owner"); code != http.StatusOK {
		t.Fatalf("owner->B = %d, want 200", code)
	}
	if code := transfer(adminB.ID, adminC.ID, "owner"); code != http.StatusOK {
		t.Fatalf("B->C = %d, want 200", code)
	}

	// Exactly one owner row must exist after the chain.
	var ownerCount int
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM tenant_memberships WHERE tenant_id = $1 AND role = 'owner'`, tn.ID).Scan(&ownerCount); err != nil {
		t.Fatalf("count owners: %v", err)
	}
	if ownerCount != 1 {
		t.Errorf("owner rows = %d, want exactly 1", ownerCount)
	}

	updatedTenant, err := a.Tenants.FindByID(context.Background(), tn.ID)
	if err != nil {
		t.Fatalf("find tenant: %v", err)
	}
	if updatedTenant.OwnerID == nil || *updatedTenant.OwnerID != adminC.ID {
		t.Errorf("tenant owner_id = %v, want %s", updatedTenant.OwnerID, adminC.ID.String())
	}
}

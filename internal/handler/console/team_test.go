package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
)

// seedTeamFixture creates a tenant with an owner and returns it together with
// the owner's membership. Callers add more members as needed.
func seedTeamFixture(t *testing.T, a *app.App, code, name string) (tenantID string, ownerID string) {
	t.Helper()
	tn := seedTenantForTest(t, a, code, name, domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "team-owner-"+code+"@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	return tn.ID.String(), owner.ID.String()
}

// =============================================================================
// GET /api/console/team
// =============================================================================

func TestHandleListTeamMembers_NoAuth(t *testing.T) {
	a := appForEnterpriseTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/console/team", nil)
	w := httptest.NewRecorder()

	HandleListTeamMembers(a).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListTeamMembers_Member_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-list-1", "Acme", domain.TenantStatusActive)
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-team-list@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/console/team", nil)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleListTeamMembers(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleListTeamMembers_IncludesStatusAndExcludesLeft(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-list-ok", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-list@test.com")
	active := seedEnterpriseUserForEnterpriseTest(t, a, "active-list@test.com")
	suspended := seedEnterpriseUserForEnterpriseTest(t, a, "suspended-list@test.com")
	left := seedEnterpriseUserForEnterpriseTest(t, a, "left-list@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, active.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, suspended.ID, domain.MembershipRoleMember, domain.MembershipStatusSuspended)
	_ = seedMembershipForAdminTest(t, a, tn.ID, left.ID, domain.MembershipRoleMember, domain.MembershipStatusLeft)

	req := httptest.NewRequest(http.MethodGet, "/api/console/team", nil)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleListTeamMembers(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Members []teamMember `json:"members"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// owner + active + suspended = 3; "left" must be omitted.
	if len(resp.Members) != 3 {
		t.Fatalf("members = %d, want 3 (left omitted), body: %s", len(resp.Members), w.Body.String())
	}

	byID := map[string]teamMember{}
	for _, m := range resp.Members {
		byID[m.ID] = m
	}

	if s := byID[suspended.ID.String()].Status; s != "suspended" {
		t.Errorf("suspended member status = %q, want suspended", s)
	}
	if s := byID[active.ID.String()].Status; s != "active" {
		t.Errorf("active member status = %q, want active", s)
	}
	if s := byID[owner.ID.String()].Status; s != "active" {
		t.Errorf("owner status = %q, want active", s)
	}
	if _, ok := byID[left.ID.String()]; ok {
		t.Error("left member should be omitted from the list")
	}
}

// =============================================================================
// PUT /api/console/team/{userId}/status
// =============================================================================

func TestHandleSuspendMember_NoAuth(t *testing.T) {
	a := appForEnterpriseTest(t)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/some-id/status", nil)
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleSuspendMember_Member_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-1", "Acme", domain.TenantStatusActive)
	member := seedEnterpriseUserForEnterpriseTest(t, a, "member-sus@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/some-id/status", body)
	req = setTenantCtxForEnterprise(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleSuspendMember_AdminSuspendsMember_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-ok", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-sus@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-sus@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+target.ID.String()+"/status", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", target.ID.String())
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	m, err := a.Memberships.FindByUserAndTenant(context.Background(), target.ID, tn.ID)
	if err != nil {
		t.Fatalf("find target: %v", err)
	}
	if m.Status != domain.MembershipStatusSuspended {
		t.Errorf("target status = %s, want suspended", m.Status)
	}
}

func TestHandleSuspendMember_AdminCannotSuspendAdmin(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-2", "Acme", domain.TenantStatusActive)
	adminA := seedEnterpriseUserForEnterpriseTest(t, a, "admin-a@test.com")
	adminB := seedEnterpriseUserForEnterpriseTest(t, a, "admin-b@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminA.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminB.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+adminB.ID.String()+"/status", body)
	req = setTenantCtxForEnterprise(req, adminA.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", adminB.ID.String())
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleSuspendMember_OwnerSuspendsAdmin_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-3", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-sus@test.com")
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-sus3@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+admin.ID.String()+"/status", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	req = chiRouteCtx(req, "userId", admin.ID.String())
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestHandleSuspendMember_CannotSuspendOwner(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-4", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-sus4@test.com")
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-sus4@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+owner.ID.String()+"/status", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", owner.ID.String())
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleSuspendMember_Self_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-self", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-self@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+admin.ID.String()+"/status", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", admin.ID.String())
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleSuspendMember_InvalidStatus_400(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-inv", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-inv@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-inv@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"banned"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+target.ID.String()+"/status", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", target.ID.String())
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleSuspendMember_NotFound_404(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-404", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-404@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/some-id/status", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", "00000000-0000-0000-0000-000000000000")
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// =============================================================================
// DELETE /api/console/team/{userId}
// =============================================================================

func TestHandleRemoveMember_AdminRemovesMember_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-rm-ok", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-rm@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-rm@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/team/"+target.ID.String(), nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", target.ID.String())
	w := httptest.NewRecorder()

	HandleRemoveMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	if _, err := a.Memberships.FindByUserAndTenant(context.Background(), target.ID, tn.ID); err == nil {
		t.Error("expected membership to be deleted")
	}
}

func TestHandleRemoveMember_AdminCannotRemoveAdmin(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-rm-2", "Acme", domain.TenantStatusActive)
	adminA := seedEnterpriseUserForEnterpriseTest(t, a, "admin-rm-a@test.com")
	adminB := seedEnterpriseUserForEnterpriseTest(t, a, "admin-rm-b@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminA.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminB.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/team/"+adminB.ID.String(), nil)
	req = setTenantCtxForEnterprise(req, adminA.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", adminB.ID.String())
	w := httptest.NewRecorder()

	HandleRemoveMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleRemoveMember_CannotRemoveOwner(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-rm-3", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-rm@test.com")
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-rm3@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/team/"+owner.ID.String(), nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", owner.ID.String())
	w := httptest.NewRecorder()

	HandleRemoveMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// =============================================================================
// PUT /api/console/team/{userId}/role
// =============================================================================

func TestHandleChangeMemberRole_Admin_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-role-1", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "admin-role@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-role@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"role":"admin"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+target.ID.String()+"/role", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", target.ID.String())
	w := httptest.NewRecorder()

	HandleChangeMemberRole(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleChangeMemberRole_Owner_OK(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-role-ok", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-role@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-role-ok@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"role":"admin"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+target.ID.String()+"/role", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	req = chiRouteCtx(req, "userId", target.ID.String())
	w := httptest.NewRecorder()

	HandleChangeMemberRole(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	m, err := a.Memberships.FindByUserAndTenant(context.Background(), target.ID, tn.ID)
	if err != nil {
		t.Fatalf("find target: %v", err)
	}
	if m.Role != domain.MembershipRoleAdmin {
		t.Errorf("target role = %s, want admin", m.Role)
	}
}

func TestHandleChangeMemberRole_CannotChangeOwner(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-role-3", "Acme", domain.TenantStatusActive)
	owner := seedEnterpriseUserForEnterpriseTest(t, a, "owner-role3@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"role":"member"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+owner.ID.String()+"/role", body)
	req = setTenantCtxForEnterprise(req, owner.ID.String(), tn.ID.String(), "owner")
	req = chiRouteCtx(req, "userId", owner.ID.String())
	w := httptest.NewRecorder()

	HandleChangeMemberRole(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// =============================================================================
// C-1: suspended admins lose administrative access
// =============================================================================

func TestHandleListTeamMembers_SuspendedAdmin_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-admin", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "suspended-admin@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusSuspended)

	req := httptest.NewRequest(http.MethodGet, "/api/console/team", nil)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	w := httptest.NewRecorder()

	HandleListTeamMembers(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleSuspendMember_SuspendedAdmin_Forbidden(t *testing.T) {
	a := appForEnterpriseTest(t)
	tn := seedTenantForTest(t, a, "team-sus-adm2", "Acme", domain.TenantStatusActive)
	admin := seedEnterpriseUserForEnterpriseTest(t, a, "suspended-admin2@test.com")
	target := seedEnterpriseUserForEnterpriseTest(t, a, "target-sus2@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, admin.ID, domain.MembershipRoleAdmin, domain.MembershipStatusSuspended)
	_ = seedMembershipForAdminTest(t, a, tn.ID, target.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"status":"suspended"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/console/team/"+target.ID.String()+"/status", body)
	req = setTenantCtxForEnterprise(req, admin.ID.String(), tn.ID.String(), "admin")
	req = chiRouteCtx(req, "userId", target.ID.String())
	w := httptest.NewRecorder()

	HandleSuspendMember(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

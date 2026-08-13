package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/go-chi/chi/v5"
)

// TestHandleListUsers_FilterByUserType verifies the ?user_type= query filter:
// personal returns only personal accounts, enterprise only enterprise ones.
func TestHandleListUsers_FilterByUserType(t *testing.T) {
	a := appForLedgerTest(t)
	// seedUserForTenantsTest creates a personal-type account (the admin actor).
	admin := seedUserForTenantsTest(t, a, "admin-users-filter@test.com", "pass", "Admin")
	_ = seedUserForLedgerTest(t, a, "personal-filter@test.com", domain.UserTypePersonal)
	_ = seedUserForLedgerTest(t, a, "enterprise-filter@test.com", domain.UserTypeEnterprise)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?user_type=personal", nil)
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()
	HandleListUsers(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []userListResponse `json:"data"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("personal total = %d, want 2 (admin + personal), body: %s", resp.Total, w.Body.String())
	}
	for _, u := range resp.Data {
		if u.UserType != string(domain.UserTypePersonal) {
			t.Errorf("user %s has user_type %q, want personal", u.Email, u.UserType)
		}
		if u.Email == "enterprise-filter@test.com" {
			t.Errorf("enterprise user leaked into personal filter")
		}
	}
}

// TestHandleListUsers_FilterByUserType_Enterprise is the enterprise side of the
// same filter: only the enterprise account comes back.
func TestHandleListUsers_FilterByUserType_Enterprise(t *testing.T) {
	a := appForLedgerTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-users-filter-e@test.com", "pass", "Admin")
	_ = seedUserForLedgerTest(t, a, "personal-filter-e@test.com", domain.UserTypePersonal)
	ent := seedUserForLedgerTest(t, a, "enterprise-filter-e@test.com", domain.UserTypeEnterprise)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?user_type=enterprise", nil)
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()
	HandleListUsers(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []userListResponse `json:"data"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("enterprise total = %d, want 1, body: %s", resp.Total, w.Body.String())
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != ent.ID.String() {
		t.Errorf("enterprise filter returned wrong rows: %+v", resp.Data)
	}
}

// TestHandleListUsers_InvalidUserType rejects unknown user_type values.
func TestHandleListUsers_InvalidUserType(t *testing.T) {
	a := appForLedgerTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-users-bad@test.com", "pass", "Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?user_type=robot", nil)
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()
	HandleListUsers(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestHandleListUsers_ExcludesDeleted verifies the admin list omits soft-deleted
// users, so a deleted personal account disappears from the management list.
func TestHandleListUsers_ExcludesDeleted(t *testing.T) {
	a := appForLedgerTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-users-del@test.com", "pass", "Admin")
	victim := seedUserForLedgerTest(t, a, "victim-del@test.com", domain.UserTypePersonal)
	_ = seedUserForLedgerTest(t, a, "survivor-del@test.com", domain.UserTypePersonal)

	// Delete the victim (soft delete → status=deleted). The handler reads the
	// chi URL param, so inject a route context like channels_test.go does.
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/admin/users/"+victim.ID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", victim.ID.String())
	reqDel = reqDel.WithContext(context.WithValue(reqDel.Context(), chi.RouteCtxKey, rctx))
	reqDel = setAdminCtxForTenants(reqDel, admin.ID.String())
	wDel := httptest.NewRecorder()
	HandleDeleteUser(a).ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d, body: %s", wDel.Code, http.StatusOK, wDel.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?user_type=personal", nil)
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()
	HandleListUsers(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Data  []userListResponse `json:"data"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// admin + survivor, victim excluded.
	if resp.Total != 2 {
		t.Fatalf("personal total = %d, want 2 (admin + survivor; victim excluded), body: %s", resp.Total, w.Body.String())
	}
	for _, u := range resp.Data {
		if u.Email == "victim-del@test.com" {
			t.Errorf("soft-deleted user leaked into admin list")
		}
	}
}

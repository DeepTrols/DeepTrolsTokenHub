package middleware

// Single-domain regression tests (任务 #13).
//
// Tenant resolution no longer consults the Host header or the (dropped)
// tenant_domains table: the gateway derives the tenant from the API key
// owner's enterprise membership, and the console derives it from the JWT
// user's membership. The platform serves exactly one domain, so these tests
// pin the account-based resolution and prove that personal users (no active
// membership) stay tenant-less without being fail-closed out of the gateway.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/pkg/keyhash"
	membershipRepo "github.com/deeptrols/api/internal/repository/membership"
	"github.com/google/uuid"
)

// mockMembershipRepo implements membership.Repository for middleware tests;
// only FindByUserID is consulted by the auth middlewares.
type mockMembershipRepo struct {
	findByUserIDFn func(ctx context.Context, userID uuid.UUID) (*domain.TenantMembership, error)
}

func (m *mockMembershipRepo) FindByUserID(ctx context.Context, userID uuid.UUID) (*domain.TenantMembership, error) {
	if m.findByUserIDFn != nil {
		return m.findByUserIDFn(ctx, userID)
	}
	return nil, membershipRepo.ErrNotFound
}
func (m *mockMembershipRepo) FindByTenantID(ctx context.Context, tenantID uuid.UUID) ([]domain.TenantMembership, error) {
	return nil, nil
}
func (m *mockMembershipRepo) FindByUserAndTenant(ctx context.Context, userID, tenantID uuid.UUID) (*domain.TenantMembership, error) {
	return nil, membershipRepo.ErrNotFound
}
func (m *mockMembershipRepo) Create(ctx context.Context, mem *domain.TenantMembership) error {
	return nil
}
func (m *mockMembershipRepo) UpdateRole(ctx context.Context, id uuid.UUID, role domain.MembershipRole) error {
	return nil
}
func (m *mockMembershipRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.MembershipStatus) error {
	return nil
}
func (m *mockMembershipRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }

// gatewayTenantRequest runs GatewayAuth for a valid key owned by userID with
// the given membership repository and returns the captured response code and
// the CtxTenantID value ("" when unset).
func gatewayTenantRequest(t *testing.T, userID uuid.UUID, memberships *mockMembershipRepo) (int, string) {
	t.Helper()

	plaintextKey := "dt-single-domain-regression-key"
	keyHash := keyhash.Hash(plaintextKey, "test-hmac-secret-32-bytes-!!!!")
	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, hash string) (*domain.APIKey, error) {
			if hash == keyHash {
				return &domain.APIKey{
					ID:     uuid.New(),
					UserID: userID,
					Status: domain.APIKeyStatusActive,
				}, nil
			}
			return nil, nil
		},
	}

	application := appWithMockRepo(repo)
	application.Memberships = memberships

	var (
		code     int
		tenantID string
	)
	handler := GatewayAuth(application)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Context().Value(CtxTenantID); v != nil {
			tenantID, _ = v.(string)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintextKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	code = w.Code
	return code, tenantID
}

func TestGatewayAuth_ActiveMembership_ResolvesTenantFromAccount(t *testing.T) {
	// The enterprise tenant comes from the key owner's membership record,
	// not from the request Host — this is the core single-domain behavior.
	userID := uuid.New()
	tenantID := uuid.New()
	memberships := &mockMembershipRepo{
		findByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TenantMembership, error) {
			if id != userID {
				t.Fatalf("membership lookup for unexpected user %s", id)
			}
			return &domain.TenantMembership{
				ID: uuid.New(), TenantID: tenantID, UserID: userID,
				Role: domain.MembershipRoleMember, Status: domain.MembershipStatusActive,
			}, nil
		},
	}

	code, gotTenant := gatewayTenantRequest(t, userID, memberships)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if gotTenant != tenantID.String() {
		t.Errorf("CtxTenantID = %q, want %q", gotTenant, tenantID.String())
	}
}

func TestGatewayAuth_NoMembership_PersonalStaysTenantless(t *testing.T) {
	// A personal user has no membership row: the gateway must serve them
	// tenant-less (shared catalog) — resolution must never fail closed here.
	userID := uuid.New()
	memberships := &mockMembershipRepo{
		findByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TenantMembership, error) {
			return nil, membershipRepo.ErrNotFound
		},
	}

	code, gotTenant := gatewayTenantRequest(t, userID, memberships)
	if code != http.StatusOK {
		t.Fatalf("personal user must not be fail-closed: status = %d, want 200", code)
	}
	if gotTenant != "" {
		t.Errorf("CtxTenantID = %q, want empty for a personal user", gotTenant)
	}
}

func TestGatewayAuth_SuspendedMembership_TenantNotSet(t *testing.T) {
	// Only an ACTIVE membership grants tenant context; a suspended one must
	// downgrade the caller to the tenant-less path, not fail the request.
	userID := uuid.New()
	memberships := &mockMembershipRepo{
		findByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TenantMembership, error) {
			return &domain.TenantMembership{
				ID: uuid.New(), TenantID: uuid.New(), UserID: userID,
				Role: domain.MembershipRoleMember, Status: domain.MembershipStatusSuspended,
			}, nil
		},
	}

	code, gotTenant := gatewayTenantRequest(t, userID, memberships)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if gotTenant != "" {
		t.Errorf("CtxTenantID = %q, want empty for a suspended membership", gotTenant)
	}
}

func TestGatewayAuth_MembershipLookupError_DegradesToTenantless(t *testing.T) {
	// Characterizes current behavior: a membership lookup failure degrades
	// the caller to the tenant-less path instead of failing the request.
	// This does not widen access — the tenant gate is purely restrictive
	// (default-open shared catalog), so degradation never grants more than
	// any personal user already has.
	userID := uuid.New()
	memberships := &mockMembershipRepo{
		findByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TenantMembership, error) {
			return nil, context.DeadlineExceeded
		},
	}

	code, gotTenant := gatewayTenantRequest(t, userID, memberships)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if gotTenant != "" {
		t.Errorf("CtxTenantID = %q, want empty on membership lookup error", gotTenant)
	}
}

func TestConsoleAuth_ActiveMembership_SetsTenantAndRole(t *testing.T) {
	// The console JWT chain resolves the same membership into tenant context
	// so enterprise pages see the correct tenant on the platform domain.
	secret := "test-jwt-single-domain-secret-32b!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	tenantID := uuid.New()
	token := generateTestJWT(t, userID, "member@corp.test", "Member", secret)

	a := consoleTestApp(cfg, &domain.User{
		ID: userID, Email: "member@corp.test", DisplayName: "Member",
		Role: "user", UserType: domain.UserTypeEnterprise, Status: domain.UserStatusActive,
	})
	a.Memberships = &mockMembershipRepo{
		findByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TenantMembership, error) {
			return &domain.TenantMembership{
				ID: uuid.New(), TenantID: tenantID, UserID: userID,
				Role: domain.MembershipRoleAdmin, Status: domain.MembershipStatusActive,
			}, nil
		},
	}

	var (
		gotTenant string
		gotRole   string
	)
	handler := ConsoleAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Context().Value(jwtutil.CtxTenantIDKey); v != nil {
			gotTenant, _ = v.(string)
		}
		if v := r.Context().Value(CtxTenantRole); v != nil {
			gotRole, _ = v.(string)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotTenant != tenantID.String() {
		t.Errorf("tenant context = %q, want %q", gotTenant, tenantID.String())
	}
	if gotRole != string(domain.MembershipRoleAdmin) {
		t.Errorf("tenant role context = %q, want %q", gotRole, domain.MembershipRoleAdmin)
	}
}

func TestConsoleAuth_NoMembership_NoTenantContext(t *testing.T) {
	// Personal console users must pass ConsoleAuth with no tenant context at
	// all — the enterprise-only context keys stay unset.
	secret := "test-jwt-personal-console-secret!!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	token := generateTestJWT(t, userID, "solo@test.local", "Solo", secret)

	a := consoleTestApp(cfg, &domain.User{
		ID: userID, Email: "solo@test.local", DisplayName: "Solo",
		Role: "user", UserType: domain.UserTypePersonal, Status: domain.UserStatusActive,
	})
	a.Memberships = &mockMembershipRepo{
		findByUserIDFn: func(ctx context.Context, id uuid.UUID) (*domain.TenantMembership, error) {
			return nil, membershipRepo.ErrNotFound
		},
	}

	var tenantPresent, rolePresent bool
	handler := ConsoleAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantPresent = r.Context().Value(jwtutil.CtxTenantIDKey) != nil
		rolePresent = r.Context().Value(CtxTenantRole) != nil
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("personal user must not be fail-closed: status = %d, want 200", w.Code)
	}
	if tenantPresent {
		t.Error("tenant context key set for a personal user, want unset")
	}
	if rolePresent {
		t.Error("tenant role context key set for a personal user, want unset")
	}
}

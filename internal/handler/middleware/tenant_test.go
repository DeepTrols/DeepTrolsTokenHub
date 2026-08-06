package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

type mockTenantRepo struct {
	findByDomainFn func(ctx context.Context, d string) (*domain.Tenant, error)
}

func (m *mockTenantRepo) FindByDomain(ctx context.Context, d string) (*domain.Tenant, error) {
	if m.findByDomainFn != nil {
		return m.findByDomainFn(ctx, d)
	}
	return nil, nil
}
func (m *mockTenantRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Tenant, error) {
	return nil, nil
}
func (m *mockTenantRepo) FindByCode(ctx context.Context, code string) (*domain.Tenant, error) {
	return nil, nil
}
func (m *mockTenantRepo) Create(ctx context.Context, t *domain.Tenant) error { return nil }
func (m *mockTenantRepo) Update(ctx context.Context, t *domain.Tenant) error { return nil }
func (m *mockTenantRepo) List(ctx context.Context) ([]domain.Tenant, error)  { return nil, nil }

type stubHandler struct {
	tenantID string
	called   bool
}

func (s *stubHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.called = true
	if val := r.Context().Value(CtxTenantID); val != nil {
		s.tenantID = val.(string)
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func TestTenantIdentification_ResolvesTenantFromHost(t *testing.T) {
	tenantID := uuid.New()
	repo := &mockTenantRepo{
		findByDomainFn: func(ctx context.Context, d string) (*domain.Tenant, error) {
			if d == "mytenant.example.com" {
				return &domain.Tenant{ID: tenantID, Code: "mytenant", Status: domain.TenantStatusActive}, nil
			}
			return nil, nil
		},
	}
	middleware := TenantIdentification(repo, nil)
	handler := &stubHandler{}
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Host = "mytenant.example.com"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if !handler.called {
		t.Fatal("next handler was not called")
	}
	if handler.tenantID != tenantID.String() {
		t.Errorf("expected tenant_id %q, got %q", tenantID.String(), handler.tenantID)
	}
}

func TestTenantIdentification_UnknownHostRejected(t *testing.T) {
	repo := &mockTenantRepo{
		findByDomainFn: func(ctx context.Context, d string) (*domain.Tenant, error) { return nil, nil },
	}
	middleware := TenantIdentification(repo, nil)
	handler := &stubHandler{}
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Host = "unknown.example.com"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Fail-closed: unknown hosts are rejected with 403.
	if handler.called {
		t.Fatal("next handler was called — unknown host should be rejected")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTenantIdentification_RepoErrorFailsClosed(t *testing.T) {
	repo := &mockTenantRepo{
		findByDomainFn: func(ctx context.Context, d string) (*domain.Tenant, error) {
			return nil, errors.New("database connection lost")
		},
	}
	middleware := TenantIdentification(repo, nil)
	handler := &stubHandler{}
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Host = "anytenant.example.com"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Fail-closed: an unverifiable tenant must NOT fall through to the
	// platform when the tenant lookup itself is failing.
	if handler.called {
		t.Fatal("next handler was called — must fail closed on tenant DB errors")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTenantIdentification_PlatformHostsPassThrough(t *testing.T) {
	repo := &mockTenantRepo{
		findByDomainFn: func(ctx context.Context, d string) (*domain.Tenant, error) { return nil, nil },
	}
	// docker service names configured via PLATFORM_HOSTS
	middleware := TenantIdentification(repo, []string{"api", "web", "worker"})
	handler := &stubHandler{}
	wrapped := middleware(handler)

	for _, host := range []string{"api", "web:8080", "[::1]:8080"} {
		req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)
		if !handler.called {
			t.Fatalf("platform host %q was rejected", host)
		}
		handler.called = false
	}
}

func TestTenantIdentification_InactiveTenantRejected(t *testing.T) {
	tenantID := uuid.New()
	repo := &mockTenantRepo{
		findByDomainFn: func(ctx context.Context, d string) (*domain.Tenant, error) {
			return &domain.Tenant{ID: tenantID, Code: "suspended", Status: domain.TenantStatusSuspended}, nil
		},
	}
	middleware := TenantIdentification(repo, nil)
	handler := &stubHandler{}
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Host = "suspended.example.com"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Fail-closed: inactive/suspended tenants are rejected.
	if handler.called {
		t.Fatal("next handler was called — inactive tenant should be rejected")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

func TestTenantIdentification_LocalhostPassesThrough(t *testing.T) {
	repo := &mockTenantRepo{
		findByDomainFn: func(ctx context.Context, d string) (*domain.Tenant, error) { return nil, nil },
	}
	middleware := TenantIdentification(repo, nil)
	handler := &stubHandler{}
	wrapped := middleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Host = "localhost:8080"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	// Localhost must always pass through with no tenant.
	if !handler.called {
		t.Fatal("next handler was not called — localhost must pass through")
	}
	if handler.tenantID != "" {
		t.Errorf("expected empty tenant_id for localhost, got %q", handler.tenantID)
	}
}

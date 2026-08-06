package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

func TestFindByDomain(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "tenant_domains", "tenants")

	tenantID := uuid.New()
	code := "domain-tenant-" + uuid.New().String()[:8]
	tenantDomain := code + ".example.com"
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO tenants (id, code, name, status) VALUES ($1, $2, $3, $4)
	`, tenantID, code, code+" name", domain.TenantStatusActive)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	_, err = repo.pool.Exec(ctx, `
		INSERT INTO tenant_domains (id, tenant_id, domain, is_primary) VALUES ($1, $2, $3, $4)
	`, uuid.New(), tenantID, tenantDomain, true)
	if err != nil {
		t.Fatalf("seed domain: %v", err)
	}

	t.Run("finds tenant by domain", func(t *testing.T) {
		found, err := repo.FindByDomain(ctx, tenantDomain)
		if err != nil {
			t.Fatalf("FindByDomain: %v", err)
		}
		if found.ID != tenantID {
			t.Errorf("ID = %s, want %s", found.ID, tenantID)
		}
	})

	t.Run("returns nil tenant for unbound domain", func(t *testing.T) {
		found, err := repo.FindByDomain(ctx, "unknown.example.com")
		if err != nil {
			t.Fatalf("FindByDomain unbound domain: %v", err)
		}
		if found != nil {
			t.Errorf("expected nil tenant, got %+v", found)
		}
	})
}

func TestTenantCRUD(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "tenant_domains", "tenants")

	code := "crud-tenant-" + uuid.New().String()[:8]
	tenant := &domain.Tenant{
		ID:          uuid.New(),
		Code:        code,
		Name:        "Test Tenant CRUD",
		Status:      domain.TenantStatusActive,
		BrandConfig: map[string]any{"logo": "https://example.com/logo.png"},
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	t.Run("creates tenant", func(t *testing.T) {
		if err := repo.Create(ctx, tenant); err != nil {
			t.Fatalf("Create: %v", err)
		}
	})

	t.Run("finds by id", func(t *testing.T) {
		found, err := repo.FindByID(ctx, tenant.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.Name != tenant.Name {
			t.Errorf("Name = %s, want %s", found.Name, tenant.Name)
		}
		if _, ok := found.BrandConfig["logo"]; !ok {
			t.Error("expected BrandConfig to have 'logo' key")
		}
	})

	t.Run("finds by code", func(t *testing.T) {
		found, err := repo.FindByCode(ctx, code)
		if err != nil {
			t.Fatalf("FindByCode: %v", err)
		}
		if found.ID != tenant.ID {
			t.Errorf("ID = %s, want %s", found.ID, tenant.ID)
		}
	})

	t.Run("updates tenant", func(t *testing.T) {
		tenant.Name = "Updated Tenant"
		tenant.Status = domain.TenantStatusSuspended
		tenant.StatusReason = "testing suspension"
		tenant.UpdatedAt = time.Now().UTC()
		if err := repo.Update(ctx, tenant); err != nil {
			t.Fatalf("Update: %v", err)
		}

		found, _ := repo.FindByID(ctx, tenant.ID)
		if found.Name != "Updated Tenant" {
			t.Errorf("Name = %s, want 'Updated Tenant'", found.Name)
		}
		if found.Status != domain.TenantStatusSuspended {
			t.Errorf("Status = %s, want suspended", found.Status)
		}
	})

	t.Run("duplicate code returns error", func(t *testing.T) {
		dup := &domain.Tenant{
			ID:        uuid.New(),
			Code:      code,
			Name:      "Duplicate",
			Status:    domain.TenantStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		err := repo.Create(ctx, dup)
		if err == nil {
			t.Error("expected error for duplicate tenant code")
		}
	})
}

func TestTenantList(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "tenant_domains", "tenants")

	for i := 0; i < 2; i++ {
		code := "list-tenant-" + uuid.New().String()[:8]
		tx := &domain.Tenant{
			ID:        uuid.New(),
			Code:      code,
			Name:      "List Tenant " + uuid.New().String()[:4],
			Status:    domain.TenantStatusActive,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := repo.Create(ctx, tx); err != nil {
			t.Fatalf("seed tenant %d: %v", i, err)
		}
	}

	t.Run("lists all tenants", func(t *testing.T) {
		tenants, err := repo.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(tenants) < 2 {
			t.Errorf("len(tenants) = %d, want at least 2", len(tenants))
		}
	})
}

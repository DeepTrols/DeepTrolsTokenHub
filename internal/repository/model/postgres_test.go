package model

import (
	"context"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

// seedModel inserts a model and returns it.
func seedModel(t *testing.T, ctx context.Context, repo *PostgresRepository, code string) domain.Model {
	t.Helper()
	m := domain.Model{
		ID:           uuid.New(),
		Code:         code,
		Provider:     "openai",
		Category:     domain.ModelCategoryChat,
		DisplayName:  code + " display",
		Status:       domain.ModelStatusActive,
		ReleaseStage: domain.ReleaseStageGA,
	}
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO models (id, code, provider, category, display_name, status, release_stage)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, m.ID, m.Code, m.Provider, m.Category, m.DisplayName, m.Status, m.ReleaseStage)
	if err != nil {
		t.Fatalf("seedModel: %v", err)
	}
	return m
}

// seedActiveChannel links an active channel to a model so it is routable.
func seedActiveChannel(t *testing.T, ctx context.Context, repo *PostgresRepository, modelID uuid.UUID) {
	t.Helper()
	_, err := repo.pool.Exec(ctx,
		`INSERT INTO channels (id, name, model_id, pool_type, status, weight, max_concurrency, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', 100, 10, NOW(), NOW())`,
		uuid.New(), "channel-"+modelID.String()[:8], modelID)
	if err != nil {
		t.Fatalf("seedActiveChannel: %v", err)
	}
}

// seedTenantModel creates a tenant and tenant_model link, returns tenantID and tmID.
func seedTenantModel(t *testing.T, ctx context.Context, repo *PostgresRepository, tenantCode string, modelID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	tenantID := uuid.New()
	_, err := repo.pool.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`,
		tenantID, tenantCode, tenantCode+" name")
	if err != nil {
		t.Fatalf("seedTenantModel tenant: %v", err)
	}
	tmID := uuid.New()
	_, err = repo.pool.Exec(ctx, `
		INSERT INTO tenant_models (id, tenant_id, model_id, is_listed, allow_payg)
		VALUES ($1, $2, $3, $4, $5)
	`, tmID, tenantID, modelID, true, true)
	if err != nil {
		t.Fatalf("seedTenantModel link: %v", err)
	}
	return tenantID, tmID
}

func TestListActive(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models")

	code1 := "gpt-4o-" + uuid.New().String()[:8]
	code2 := "claude-3-" + uuid.New().String()[:8]
	m1 := seedModel(t, ctx, repo, code1)
	m2 := seedModel(t, ctx, repo, code2)
	seedActiveChannel(t, ctx, repo, m1.ID)
	seedActiveChannel(t, ctx, repo, m2.ID)

	// create an inactive model
	inactiveID := uuid.New()
	inactiveCode := "inactive-m-" + uuid.New().String()[:8]
	_, _ = repo.pool.Exec(ctx, `INSERT INTO models (id, code, provider, category, display_name, status, release_stage) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		inactiveID, inactiveCode, "openai", "chat", "inactive", "inactive", "GA")

	_ = m1
	_ = m2

	t.Run("returns only active models", func(t *testing.T) {
		models, err := repo.ListActive(ctx)
		if err != nil {
			t.Fatalf("ListActive: %v", err)
		}
		if len(models) < 2 {
			t.Errorf("expected at least 2 active models, got %d", len(models))
		}
		for _, m := range models {
			if m.Status != domain.ModelStatusActive && m.Status != domain.ModelStatusBeta {
				t.Errorf("found inactive model %s with status %s", m.Code, m.Status)
			}
		}
	})
}

func TestFindByCode(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models")

	code := "gpt-4o-test-" + uuid.New().String()[:8]
	m := seedModel(t, ctx, repo, code)

	t.Run("finds by code", func(t *testing.T) {
		found, err := repo.FindByCode(ctx, m.Code)
		if err != nil {
			t.Fatalf("FindByCode: %v", err)
		}
		if found.ID != m.ID {
			t.Errorf("ID = %s, want %s", found.ID, m.ID)
		}
	})

	t.Run("returns error for unknown code", func(t *testing.T) {
		_, err := repo.FindByCode(ctx, "bogus-code-xxx")
		if err == nil {
			t.Error("expected error for unknown code")
		}
	})
}

func TestListByTenant(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models", "tenants")

	code := "listbytenant-" + uuid.New().String()[:8]
	m := seedModel(t, ctx, repo, code)
	tenantID, _ := seedTenantModel(t, ctx, repo, "tmt-"+uuid.New().String()[:8], m.ID)

	// add pricing
	pid := uuid.New()
	repo.pool.Exec(ctx, `
		INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, is_active)
		VALUES ($1,$2,'chat','input','token','0.000005',true)
	`, pid, m.ID)

	t.Run("lists by tenant with model view", func(t *testing.T) {
		views, err := repo.ListByTenant(ctx, &tenantID)
		if err != nil {
			t.Fatalf("ListByTenant: %v", err)
		}
		if len(views) == 0 {
			t.Error("expected at least 1 tenant model view")
		}
		found := false
		for _, v := range views {
			if v.Model.Code == m.Code {
				found = true
				if v.TenantModel == nil {
					t.Error("expected TenantModel to be non-nil")
				}
			}
		}
		if !found {
			t.Errorf("expected model %s in results", m.Code)
		}
	})

	t.Run("lists without tenant filter (platform models)", func(t *testing.T) {
		views, err := repo.ListByTenant(ctx, nil)
		if err != nil {
			t.Fatalf("ListByTenant: %v", err)
		}
		if len(views) == 0 {
			t.Error("expected at least 1 model")
		}
	})
}

func TestGetTenantModel(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models", "tenants")

	code := "gpt-4o-get-" + uuid.New().String()[:8]
	m := seedModel(t, ctx, repo, code)
	tenantID, _ := seedTenantModel(t, ctx, repo, "tmt-get-"+uuid.New().String()[:8], m.ID)

	t.Run("finds tenant model", func(t *testing.T) {
		tm, err := repo.GetTenantModel(ctx, tenantID, m.Code)
		if err != nil {
			t.Fatalf("GetTenantModel: %v", err)
		}
		if tm.TenantID != tenantID {
			t.Errorf("TenantID = %s, want %s", tm.TenantID, tenantID)
		}
	})

	t.Run("returns error when not linked", func(t *testing.T) {
		unknownTenant := uuid.New()
		_, err := repo.GetTenantModel(ctx, unknownTenant, m.Code)
		if err == nil {
			t.Error("expected error for unlinked tenant-model")
		}
	})
}

func TestFindByID(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models")

	code := "findbyid-" + uuid.New().String()[:8]
	m := seedModel(t, ctx, repo, code)

	t.Run("finds model by id", func(t *testing.T) {
		found, err := repo.FindByID(ctx, m.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found == nil {
			t.Fatal("expected non-nil model")
		}
		if found.Code != m.Code {
			t.Errorf("Code = %s, want %s", found.Code, m.Code)
		}
		if found.Provider != m.Provider {
			t.Errorf("Provider = %s, want %s", found.Provider, m.Provider)
		}
	})

	t.Run("returns error for unknown id", func(t *testing.T) {
		_, err := repo.FindByID(ctx, uuid.New())
		if err == nil {
			t.Error("expected error for unknown id")
		}
	})
}

func TestFindByModelPricing(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models", "tenants")

	code := "pricing-test-" + uuid.New().String()[:8]
	m := seedModel(t, ctx, repo, code)

	tenantID := uuid.New()
	repo.pool.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`,
		tenantID, "pricing-tenant-"+uuid.New().String()[:8], "pricing tenant")

	// platform pricing (tenant_id IS NULL)
	p1 := uuid.New()
	repo.pool.Exec(ctx, `
		INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, is_active)
		VALUES ($1,$2,'chat','input','token','0.000004',true)
	`, p1, m.ID)

	// tenant-specific pricing
	p2 := uuid.New()
	repo.pool.Exec(ctx, `
		INSERT INTO model_pricing (id, model_id, tenant_id, request_type, pricing_dimension, unit_name, unit_price, is_active)
		VALUES ($1,$2,$3,'chat','output','token','0.000010',true)
	`, p2, m.ID, tenantID)

	t.Run("finds pricing for model", func(t *testing.T) {
		pricing, err := repo.FindByModel(ctx, m.ID, nil)
		if err != nil {
			t.Fatalf("FindByModel: %v", err)
		}
		if len(pricing) < 2 {
			t.Errorf("expected at least 2 pricing rows, got %d", len(pricing))
		}
	})

	t.Run("finds pricing filtered by tenant", func(t *testing.T) {
		pricing, err := repo.FindByModel(ctx, m.ID, &tenantID)
		if err != nil {
			t.Fatalf("FindByModel: %v", err)
		}
		found := false
		for _, p := range pricing {
			if p.TenantID != nil && *p.TenantID == tenantID {
				found = true
			}
		}
		if !found {
			t.Error("expected at least one tenant-specific pricing row")
		}
	})

	t.Run("returns empty for unknown model", func(t *testing.T) {
		pricing, err := repo.FindByModel(ctx, uuid.New(), nil)
		if err != nil {
			t.Fatalf("FindByModel: %v", err)
		}
		if len(pricing) != 0 {
			t.Errorf("expected empty, got %d", len(pricing))
		}
	})
}

func TestFindByModel_ScansPriceVersion(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models")

	m := seedModel(t, ctx, repo, "pv-"+uuid.New().String()[:8])
	pricingID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, upstream_cost, is_active, price_version)
		VALUES ($1, $2, 'chat', 'input', 'token', '0.000015', 'CNY', '0.000010', true, 7)
	`, pricingID, m.ID)
	if err != nil {
		t.Fatalf("seed pricing: %v", err)
	}

	pricing, err := repo.FindByModel(ctx, m.ID, nil)
	if err != nil {
		t.Fatalf("FindByModel: %v", err)
	}
	if len(pricing) != 1 {
		t.Fatalf("pricing rows = %d, want 1", len(pricing))
	}
	if pricing[0].ID != pricingID {
		t.Errorf("ID = %s, want %s", pricing[0].ID, pricingID)
	}
	if pricing[0].PriceVersion != 7 {
		t.Errorf("PriceVersion = %d, want 7", pricing[0].PriceVersion)
	}
}

func TestFindByModel_ScansPriceTypeAndPeriod(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "model_pricing", "tenant_models", "models")

	m := seedModel(t, ctx, repo, "pt-"+uuid.New().String()[:8])

	// Explicit cost/peak row.
	costID := uuid.New()
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, is_active, price_type, period)
		VALUES ($1, $2, 'chat', 'input', '1K tokens', '0.003', 'CNY', true, 'cost', 'peak')
	`, costID, m.ID); err != nil {
		t.Fatalf("seed cost pricing: %v", err)
	}
	// Defaults for legacy rows: sell / off_peak.
	sellID := uuid.New()
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, is_active)
		VALUES ($1, $2, 'chat', 'output', '1K tokens', '0.009', 'CNY', true)
	`, sellID, m.ID); err != nil {
		t.Fatalf("seed sell pricing: %v", err)
	}

	pricing, err := repo.FindByModel(ctx, m.ID, nil)
	if err != nil {
		t.Fatalf("FindByModel: %v", err)
	}
	if len(pricing) != 2 {
		t.Fatalf("pricing rows = %d, want 2", len(pricing))
	}
	byID := map[uuid.UUID]domain.ModelPricing{}
	for _, p := range pricing {
		byID[p.ID] = p
	}
	if p := byID[costID]; p.PriceType != "cost" || p.Period != "peak" {
		t.Errorf("cost row type/period = %q/%q, want cost/peak", p.PriceType, p.Period)
	}
	if p := byID[sellID]; p.PriceType != "sell" || p.Period != "off_peak" {
		t.Errorf("legacy row type/period = %q/%q, want sell/off_peak", p.PriceType, p.Period)
	}
}

package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

// appForQuotasTest creates a minimal App wired for quota pool tests.
func appForQuotasTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-quotas-32byte",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Healthy: true,
	}
}

// seedQuotaPools inserts quota pool records for testing.
func seedQuotaPools(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()

	// Create a tenant first (FK constraint)
	tenantID := uuid.New()
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at)
		 VALUES ($1, 'test-tenant', 'Test Tenant', 'active', NOW(), NOW())`,
		tenantID,
	)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	// Create a model first (FK constraint)
	modelID := uuid.New()
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
		 VALUES ($1, 'gpt-4o', 'openai', 'chat', 'GPT-4o', 'active', 'GA', NOW(), NOW())`,
		modelID,
	)
	if err != nil {
		t.Fatalf("insert model: %v", err)
	}

	// Insert quota pools
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO quota_pools (id, tenant_id, model_id, dimension, total_amount, allocated_amount, used_amount, unit_name, created_at, updated_at)
		 VALUES ($1, $2, $3, 'token', 1000000, 500000, 200000, 'token', NOW(), NOW())`,
		uuid.New(), tenantID, modelID,
	)
	if err != nil {
		t.Fatalf("insert quota pool 1: %v", err)
	}

	_, err = a.Pool.Exec(ctx,
		`INSERT INTO quota_pools (id, tenant_id, model_id, dimension, total_amount, allocated_amount, used_amount, unit_name, created_at, updated_at)
		 VALUES ($1, $2, $3, 'token', 2000000, 0, 0, 'token', NOW(), NOW())`,
		uuid.New(), tenantID, modelID,
	)
	if err != nil {
		t.Fatalf("insert quota pool 2: %v", err)
	}
}

// =============================================================================
// HandleListQuotaPools Tests
// =============================================================================

func TestHandleListQuotaPools_Success(t *testing.T) {
	a := appForQuotasTest(t)
	seedQuotaPools(t, a)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/quotas", nil)
	w := httptest.NewRecorder()

	handler := HandleListQuotaPools(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			ID              string `json:"id"`
			Dimension       string `json:"dimension"`
			TotalAmount     int64  `json:"total_amount"`
			AllocatedAmount int64  `json:"allocated_amount"`
			UsedAmount      int64  `json:"used_amount"`
			UnitName        string `json:"unit_name"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 quota pools, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
}

func TestHandleListQuotaPools_EmptyList(t *testing.T) {
	a := appForQuotasTest(t)
	// No quota pools seeded

	req := httptest.NewRequest(http.MethodGet, "/api/admin/quotas", nil)
	w := httptest.NewRecorder()

	handler := HandleListQuotaPools(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty array, got %d items", len(resp.Data))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}

func TestHandleListQuotaPools_ResponseContentTypeJSON(t *testing.T) {
	a := appForQuotasTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/quotas", nil)
	w := httptest.NewRecorder()

	handler := HandleListQuotaPools(a)
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

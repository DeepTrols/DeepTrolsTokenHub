package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/repository/quota"
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
		Quotas:  quota.NewPostgresRepository(pool),
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

// A pool scoped to a tenant but not a model (留空 = 所有模型) must list without
// error. The list query scans the nullable model_id UUID column into a string,
// so NULL must be coalesced to an empty string rather than failing the scan.
func TestHandleListQuotaPools_WithNullModel(t *testing.T) {
	a := appForQuotasTest(t)
	ctx := context.Background()

	tenantID := uuid.New()
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at)
		 VALUES ($1, 'list-null-model', 'Null Model Tenant', 'active', NOW(), NOW())`,
		tenantID,
	)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}

	// tenant-scoped, model NULL (tenant_id is NOT NULL per schema)
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO quota_pools (id, tenant_id, model_id, dimension, total_amount, allocated_amount, used_amount, unit_name, created_at, updated_at)
		 VALUES ($1, $2, NULL, 'token', 1000000, 0, 0, 'token', NOW(), NOW())`,
		uuid.New(), tenantID,
	)
	if err != nil {
		t.Fatalf("insert tenant pool with NULL model: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/quotas", nil)
	w := httptest.NewRecorder()
	HandleListQuotaPools(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			TenantID string `json:"tenant_id"`
			ModelID  string `json:"model_id"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 quota pool, got %d", len(resp.Data))
	}
	// NULL model must come back as empty string, not an error.
	if got := resp.Data[0].ModelID; got != "" {
		t.Errorf("model_id = %q, want empty for NULL model", got)
	}
	if got := resp.Data[0].TenantID; got == "" {
		t.Errorf("tenant_id = %q, want the pool's tenant UUID", got)
	}
}

// tenant_id is NOT NULL in the schema, so creating a pool without one must be
// rejected with a 400 rather than surfacing an opaque 500 from the insert.
func TestHandleCreateQuotaPool_RequiresTenantID(t *testing.T) {
	a := appForQuotasTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/quotas",
		strings.NewReader(`{"model_id":null,"dimension":"token","total_amount":1000,"unit_name":"token"}`))
	w := httptest.NewRecorder()
	HandleCreateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// tenant_id present but empty must be rejected too, not only a missing field.
func TestHandleCreateQuotaPool_RequiresTenantID_EmptyString(t *testing.T) {
	a := appForQuotasTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/quotas",
		strings.NewReader(`{"tenant_id":"","dimension":"token","total_amount":1000,"unit_name":"token"}`))
	w := httptest.NewRecorder()
	HandleCreateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// A model_id that is present but not a UUID must be rejected instead of being
// silently coerced to NULL (which would scope the pool to the whole tenant).
func TestHandleCreateQuotaPool_InvalidModelID(t *testing.T) {
	a := appForQuotasTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/quotas",
		strings.NewReader(`{"tenant_id":"`+uuid.New().String()+`","model_id":"gpt-4o","dimension":"token","total_amount":1000,"unit_name":"token"}`))
	w := httptest.NewRecorder()
	HandleCreateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// A retried allocation must not grant quota twice: the repository replays the
// recorded result for a repeated (pool, user, amount) idempotency key, so the
// pool's allocated counter advances only once.
func TestHandleAllocateQuota_RetryIsIdempotent(t *testing.T) {
	a := appForQuotasTest(t)
	ctx := context.Background()

	tenantID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at)
		 VALUES ($1, 'alloc-idem', 'Alloc Idem Tenant', 'active', NOW(), NOW())`,
		tenantID); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	poolID := seedPoolForTeamTest(t, a, tenantID, 1_000_000)
	// quota_allocations.user_id has an FK to users(id), so the recipient must be
	// a real user row, not a bare UUID.
	userID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, status, created_at, updated_at)
		 VALUES ($1, 'alloc-idem@test.com', 'x', 'Alloc Idem User', 'active', NOW(), NOW())`,
		userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	allocate := func() *httptest.ResponseRecorder {
		body := strings.NewReader(`{"user_id":"` + userID.String() + `","amount":10000}`)
		req := httptest.NewRequest(http.MethodPost, "/api/admin/quotas/"+poolID.String()+"/allocate", body)
		req = chiRouteCtx(req, "id", poolID.String())
		w := httptest.NewRecorder()
		HandleAllocateQuota(a).ServeHTTP(w, req)
		return w
	}

	first := allocate()
	second := allocate()
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("allocate statuses = %d, %d, want 201, 201; first=%s second=%s",
			first.Code, second.Code, first.Body.String(), second.Body.String())
	}
	// Both responses must reference the same allocation record.
	if first.Body.String() != second.Body.String() {
		t.Errorf("retried allocation returned a different id: %s vs %s",
			first.Body.String(), second.Body.String())
	}

	var allocated int64
	if err := a.Pool.QueryRow(ctx,
		`SELECT allocated_amount FROM quota_pools WHERE id = $1`, poolID).Scan(&allocated); err != nil {
		t.Fatalf("query pool allocated_amount: %v", err)
	}
	if allocated != 10000 {
		t.Errorf("pool allocated_amount = %d, want 10000 (granted once, not twice)", allocated)
	}
}

// A pool must belong to a valid tenant UUID.
func TestHandleCreateQuotaPool_InvalidTenantID(t *testing.T) {
	a := appForQuotasTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/quotas",
		strings.NewReader(`{"tenant_id":"not-a-uuid","dimension":"token","total_amount":1000,"unit_name":"token"}`))
	w := httptest.NewRecorder()
	HandleCreateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// =============================================================================
// HandleUpdateQuotaPool Tests
// =============================================================================

func seedQuotaPoolHandler(t *testing.T, a *app.App, tenantID uuid.UUID, total int64) uuid.UUID {
	t.Helper()
	poolID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO quota_pools (id, tenant_id, dimension, total_amount, allocated_amount, used_amount, unit_name, created_at, updated_at)
		 VALUES ($1, $2, 'token', $3, 0, 0, 'token', NOW(), NOW())`,
		poolID, tenantID, total,
	)
	if err != nil {
		t.Fatalf("insert quota pool: %v", err)
	}
	return poolID
}

func TestHandleUpdateQuotaPool_Success(t *testing.T) {
	a := appForQuotasTest(t)
	tenantID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at)
		 VALUES ($1, 'upd-quota', 'Update Quota Tenant', 'active', NOW(), NOW())`, tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	poolID := seedQuotaPoolHandler(t, a, tenantID, 100000)

	body := strings.NewReader(`{"total_amount":250000,"unit_name":"chars","dimension":"token"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/quotas/"+poolID.String(), body)
	req = chiRouteCtx(req, "id", poolID.String())
	w := httptest.NewRecorder()
	HandleUpdateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		TotalAmount int64  `json:"total_amount"`
		UnitName    string `json:"unit_name"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TotalAmount != 250000 || resp.UnitName != "chars" {
		t.Errorf("resp = %+v, want total=250000 unit=chars", resp)
	}
}

func TestHandleUpdateQuotaPool_NotFound(t *testing.T) {
	a := appForQuotasTest(t)

	poolID := uuid.New().String()
	body := strings.NewReader(`{"total_amount":1000,"unit_name":"token","dimension":"token"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/quotas/"+poolID, body)
	req = chiRouteCtx(req, "id", poolID)
	w := httptest.NewRecorder()
	HandleUpdateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// Shrinking a pool below what is already allocated must surface as a 400 with a
// readable message, not a generic 500.
func TestHandleUpdateQuotaPool_BelowAllocatedRejected(t *testing.T) {
	a := appForQuotasTest(t)
	ctx := context.Background()

	tenantID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at)
		 VALUES ($1, 'upd-quota-2', 'Update Quota Tenant 2', 'active', NOW(), NOW())`, tenantID); err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	poolID := seedQuotaPoolHandler(t, a, tenantID, 100000)
	userID := uuid.New()
	if _, err := a.Pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, status, created_at, updated_at)
		 VALUES ($1, 'upd-quota@test.com', 'x', 'Upd Quota User', 'active', NOW(), NOW())`, userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	allocReq := httptest.NewRequest(http.MethodPost, "/api/admin/quotas/"+poolID.String()+"/allocate",
		strings.NewReader(`{"user_id":"`+userID.String()+`","amount":60000}`))
	allocReq = chiRouteCtx(allocReq, "id", poolID.String())
	aw := httptest.NewRecorder()
	HandleAllocateQuota(a).ServeHTTP(aw, allocReq)
	if aw.Code != http.StatusCreated {
		t.Fatalf("seed allocate status = %d, want 201, body: %s", aw.Code, aw.Body.String())
	}

	body := strings.NewReader(`{"total_amount":50000,"unit_name":"token","dimension":"token"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/quotas/"+poolID.String(), body)
	req = chiRouteCtx(req, "id", poolID.String())
	w := httptest.NewRecorder()
	HandleUpdateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "allocated") {
		t.Errorf("body should mention allocated amount, got: %s", w.Body.String())
	}
}

func TestHandleUpdateQuotaPool_InvalidPoolID(t *testing.T) {
	a := appForQuotasTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/quotas/not-a-uuid",
		strings.NewReader(`{"total_amount":1000,"unit_name":"token","dimension":"token"}`))
	req = chiRouteCtx(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	HandleUpdateQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// =============================================================================
// HandleDeleteQuotaPool Tests
// =============================================================================

func TestHandleDeleteQuotaPool_Success(t *testing.T) {
	a := appForQuotasTest(t)
	tenantID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO tenants (id, code, name, status, created_at, updated_at)
		 VALUES ($1, 'del-quota', 'Delete Quota Tenant', 'active', NOW(), NOW())`, tenantID)
	if err != nil {
		t.Fatalf("insert tenant: %v", err)
	}
	poolID := seedQuotaPoolHandler(t, a, tenantID, 100000)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/quotas/"+poolID.String(), nil)
	req = chiRouteCtx(req, "id", poolID.String())
	w := httptest.NewRecorder()
	HandleDeleteQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"deleted"`) {
		t.Errorf("body = %s, want status deleted", w.Body.String())
	}

	// Pool is gone from the list.
	var count int
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM quota_pools WHERE id = $1`, poolID).Scan(&count)
	if count != 0 {
		t.Errorf("quota_pools rows = %d, want 0 after delete", count)
	}
}

func TestHandleDeleteQuotaPool_NotFound(t *testing.T) {
	a := appForQuotasTest(t)

	poolID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/quotas/"+poolID, nil)
	req = chiRouteCtx(req, "id", poolID)
	w := httptest.NewRecorder()
	HandleDeleteQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleDeleteQuotaPool_InvalidPoolID(t *testing.T) {
	a := appForQuotasTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/quotas/not-a-uuid", nil)
	req = chiRouteCtx(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()
	HandleDeleteQuotaPool(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

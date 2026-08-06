package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

// appForReconciliationTest creates a minimal App wired for reconciliation tests.
func appForReconciliationTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-recon-32byte",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Healthy: true,
	}
}

// seedReconciliationRuns inserts reconciliation run records for testing.
func seedReconciliationRuns(t *testing.T, a *app.App) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	// Insert two reconciliation runs
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO reconciliation_runs (id, level, period_start, period_end, total_requests, diff_count, status, report, created_at, completed_at)
		 VALUES ($1, 'L1', $2, $3, 100, 5, 'completed', '{"summary":"ok"}', $4, $5)`,
		uuid.New(), now.Add(-24*time.Hour), now.Add(-23*time.Hour), now.Add(-24*time.Hour), now.Add(-23*time.Hour),
	)
	if err != nil {
		t.Fatalf("insert reconciliation run 1: %v", err)
	}

	_, err = a.Pool.Exec(ctx,
		`INSERT INTO reconciliation_runs (id, level, period_start, period_end, total_requests, diff_count, status, report, created_at, completed_at)
		 VALUES ($1, 'L0', $2, $3, 200, 0, 'running', '{}', $4, NULL)`,
		uuid.New(), now.Add(-2*time.Hour), now.Add(-1*time.Hour), now.Add(-2*time.Hour),
	)
	if err != nil {
		t.Fatalf("insert reconciliation run 2: %v", err)
	}
}

// =============================================================================
// HandleListReconciliationRuns Tests
// =============================================================================

func TestHandleListReconciliationRuns_Success(t *testing.T) {
	a := appForReconciliationTest(t)
	seedReconciliationRuns(t, a)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reconciliation", nil)
	w := httptest.NewRecorder()

	handler := HandleListReconciliationRuns(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			ID          string `json:"id"`
			RunType     string `json:"run_type"`
			Status      string `json:"status"`
			StartedAt   string `json:"started_at"`
			TotalLogs   int    `json:"total_usage_logs"`
			Matched     int    `json:"matched_count"`
			DiffCount   int    `json:"diff_count"`
			PeriodStart string `json:"period_start"`
			PeriodEnd   string `json:"period_end"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 reconciliation runs, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	// Most recent first (created_at DESC)
	if resp.Data[0].Status != "running" {
		t.Errorf("first entry status = %s, want 'running'", resp.Data[0].Status)
	}
	if resp.Data[1].Status != "completed" {
		t.Errorf("second entry status = %s, want 'completed'", resp.Data[1].Status)
	}
}

func TestHandleListReconciliationRuns_EmptyList(t *testing.T) {
	a := appForReconciliationTest(t)
	// No reconciliation runs seeded

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reconciliation", nil)
	w := httptest.NewRecorder()

	handler := HandleListReconciliationRuns(a)
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

func TestHandleListReconciliationRuns_MatchedCountComputed(t *testing.T) {
	a := appForReconciliationTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Insert a run with known values: total_requests=100, diff_count=5 => matched=95
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO reconciliation_runs (id, level, period_start, period_end, total_requests, diff_count, status, report, created_at, completed_at)
		 VALUES ($1, 'L2', $2, $3, 100, 5, 'completed', '{}', $4, $5)`,
		uuid.New(), now.Add(-24*time.Hour), now.Add(-23*time.Hour), now.Add(-24*time.Hour), now.Add(-23*time.Hour),
	)
	if err != nil {
		t.Fatalf("insert reconciliation run: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reconciliation", nil)
	w := httptest.NewRecorder()

	handler := HandleListReconciliationRuns(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []struct {
			Matched int `json:"matched_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Data))
	}
	// total_requests=100, diff_count=5 => matched_count=95
	if resp.Data[0].Matched != 95 {
		t.Errorf("matched_count = %d, want 95", resp.Data[0].Matched)
	}
}

func TestHandleListReconciliationRuns_Max50Entries(t *testing.T) {
	a := appForReconciliationTest(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Insert 55 reconciliation runs
	for i := 0; i < 55; i++ {
		offset := time.Duration(i) * time.Hour
		_, err := a.Pool.Exec(ctx,
			`INSERT INTO reconciliation_runs (id, level, period_start, period_end, total_requests, diff_count, status, report, created_at, completed_at)
			 VALUES ($1, 'L1', $2, $3, 1, 0, 'completed', '{}', $4, $5)`,
			uuid.New(),
			now.Add(-offset-1*time.Hour),
			now.Add(-offset),
			now.Add(-offset-1*time.Hour),
			now.Add(-offset),
		)
		if err != nil {
			t.Fatalf("insert reconciliation run %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reconciliation", nil)
	w := httptest.NewRecorder()

	handler := HandleListReconciliationRuns(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) > 50 {
		t.Errorf("expected at most 50 entries, got %d", len(resp.Data))
	}
	if len(resp.Data) < 50 {
		t.Errorf("expected exactly 50 entries (LIMIT 50), got %d", len(resp.Data))
	}
}

func TestHandleListReconciliationRuns_ResponseContentTypeJSON(t *testing.T) {
	a := appForReconciliationTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reconciliation", nil)
	w := httptest.NewRecorder()

	handler := HandleListReconciliationRuns(a)
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

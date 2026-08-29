package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestHandleReconciliationSummary_IncludesL2(t *testing.T) {
	a := appForWalletTest(t)
	seedAdmin := seedUserForWalletTest(t, a, "recon-admin@example.com", "pass", "Recon Admin")

	report := `{"L2":{"usage_logs":3,"with_charge":2,"with_evidence":1,"both_missing":1,"balanced":false}}`
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO reconciliation_runs (id, level, period_start, period_end, total_requests, diff_count, status, report, created_at, completed_at)
		 VALUES ($1, 'L1', NOW() - INTERVAL '1 hour', NOW(), 3, 2, 'completed', $2::jsonb, NOW(), NOW())`,
		uuid.New(), report); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reconciliation/summary", nil)
	req = setAdminContext(req, seedAdmin.ID.String())
	w := httptest.NewRecorder()
	HandleReconciliationSummary(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		L2 struct {
			UsageLogs    int64 `json:"usage_logs"`
			WithCharge   int64 `json:"with_charge"`
			WithEvidence int64 `json:"with_evidence"`
			BothMissing  int64 `json:"both_missing"`
			Balanced     bool  `json:"balanced"`
			Available    bool  `json:"available"`
		} `json:"l2"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !resp.L2.Available {
		t.Fatal("l2.available should be true with a completed run")
	}
	if resp.L2.UsageLogs != 3 || resp.L2.WithCharge != 2 || resp.L2.WithEvidence != 1 || resp.L2.BothMissing != 1 {
		t.Fatalf("l2 = %+v, want 3/2/1/1", resp.L2)
	}
	if resp.L2.Balanced {
		t.Error("l2.balanced should be false when coverage is incomplete")
	}
}

func TestHandleReconciliationSummary_NoRuns(t *testing.T) {
	a := appForWalletTest(t)
	seedAdmin := seedUserForWalletTest(t, a, "recon-empty@example.com", "pass", "Recon Empty")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/reconciliation/summary", nil)
	req = setAdminContext(req, seedAdmin.ID.String())
	w := httptest.NewRecorder()
	HandleReconciliationSummary(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		L2 struct {
			Available bool `json:"available"`
		} `json:"l2"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.L2.Available {
		t.Fatal("l2.available should be false when no completed run exists")
	}
}

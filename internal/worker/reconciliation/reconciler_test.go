package reconciliation

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/deeptrols/api/internal/pkg/metrics"
)

// mockRow implements pgx.Row by delegating Scan to a function.
type mockRow struct {
	scanFn func(dest ...any) error
}

func (r *mockRow) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	return nil
}

// mockRows implements pgx.Rows with configurable Next/Scan.
type mockRows struct {
	items  [][]any
	idx    int
	scanFn func(dest ...any) error
	errFn  func() error
}

func (r *mockRows) Close() {}
func (r *mockRows) Err() error {
	if r.errFn != nil {
		return r.errFn()
	}
	return nil
}
func (r *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *mockRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *mockRows) Values() ([]any, error)                       { return nil, nil }
func (r *mockRows) RawValues() [][]byte                          { return nil }
func (r *mockRows) Conn() *pgx.Conn                              { return nil }

func (r *mockRows) Next() bool {
	if r.idx < len(r.items) {
		r.idx++
		return true
	}
	return false
}

func (r *mockRows) Scan(dest ...any) error {
	if r.scanFn != nil {
		return r.scanFn(dest...)
	}
	if r.idx > 0 && r.idx-1 < len(r.items) {
		item := r.items[r.idx-1]
		for i, d := range dest {
			if i >= len(item) {
				continue
			}
			switch p := d.(type) {
			case *string:
				if s, ok := item[i].(string); ok {
					*p = s
				}
			case *int:
				if n, ok := item[i].(int); ok {
					*p = n
				}
			}
		}
	}
	return nil
}

// mockDB implements dbPool with programmable behaviors.
type mockDB struct {
	queryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
	queryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	execFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (m *mockDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, sql, args...)
	}
	return &mockRows{}, nil
}

func (m *mockDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if m.execFn != nil {
		return m.execFn(ctx, sql, args...)
	}
	return pgconn.CommandTag{}, nil
}

func (m *mockDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if m.queryRowFn != nil {
		return m.queryRowFn(ctx, sql, args...)
	}
	return &mockRow{}
}

// countScan assigns sequential ints to two *int scan destinations (usage, charge).
func countScanFn() func(dest ...any) error {
	call := 0
	return func(dest ...any) error {
		call++
		if p, ok := dest[0].(*int); ok {
			*p = call
		}
		return nil
	}
}

// collectExec returns an execFn that records each SQL statement for assertions.
func collectExec(execSQLs *[]string) func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
		*execSQLs = append(*execSQLs, sql)
		return pgconn.CommandTag{}, nil
	}
}

func containsSQL(sqls []string, fragment string) bool {
	for _, s := range sqls {
		if len(s) >= len(fragment) && containsFragment(s, fragment) {
			return true
		}
	}
	return false
}

func containsFragment(s, frag string) bool {
	for i := 0; i+len(frag) <= len(s); i++ {
		if s[i:i+len(frag)] == frag {
			return true
		}
	}
	return false
}

func TestRun_SuccessNoOrphans(t *testing.T) {
	var execSQLs []string
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: countScanFn()}
		},
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{}, nil
		},
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if !containsSQL(execSQLs, "INSERT INTO reconciliation_runs") {
		t.Error("createRun INSERT not executed")
	}
	if !containsSQL(execSQLs, "status = 'completed'") {
		t.Error("completeRun UPDATE not executed")
	}
}

func TestRun_WithOrphans(t *testing.T) {
	orphanID := uuid.New().String()
	var execSQLs []string
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: countScanFn()}
		},
		queryFn: func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
			// findOrphaned returns one orphan id.
			return &mockRows{items: [][]any{{orphanID}}}, nil
		},
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if !containsSQL(execSQLs, "INSERT INTO reconciliation_diffs") {
		t.Error("createDiff INSERT not executed for orphan")
	}
	if !containsSQL(execSQLs, "status = 'completed'") {
		t.Error("completeRun UPDATE not executed")
	}
}

func TestRun_CountLogsError_MarksFailed(t *testing.T) {
	var execSQLs []string
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: func(_ ...any) error {
				return errors.New("count failed")
			}}
		},
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when countLogs fails")
	}

	if !containsSQL(execSQLs, "status = 'failed'") {
		t.Error("markRunFailed UPDATE with status='failed' not executed")
	}
}

func TestRun_CreateRunError(t *testing.T) {
	db := &mockDB{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, errors.New("create run failed")
		},
	}
	r := &Reconciler{pool: db}

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when createRun fails")
	}
}

func TestRun_FindOrphanedError_StillCompletes(t *testing.T) {
	var execSQLs []string
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: countScanFn()}
		},
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return nil, errors.New("orphan query failed")
		},
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: findOrphaned error should not fail the run, got: %v", err)
	}

	// The run must still complete (report L0) even when the orphan scan fails.
	if !containsSQL(execSQLs, "status = 'completed'") {
		t.Error("completeRun UPDATE not executed after findOrphaned failure")
	}
}

func TestRun_CreateDiffError_FailsRun(t *testing.T) {
	orphanID := uuid.New().String()
	db := &mockDB{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &mockRow{scanFn: countScanFn()}
		},
		queryFn: func(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
			return &mockRows{items: [][]any{{orphanID}}}, nil
		},
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if containsFragment(sql, "INSERT INTO reconciliation_diffs") {
				return pgconn.CommandTag{}, errors.New("diff insert failed")
			}
			return pgconn.CommandTag{}, nil
		},
	}
	r := &Reconciler{pool: db}

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when createDiff fails to persist")
	}
}

// queryRowsByFragment returns a queryFn that matches the SQL by fragment and
// delegates to the matching handler, defaulting to empty rows.
func queryRowsByFragment(
	handlers map[string]func() (pgx.Rows, error),
) func(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return func(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
		for frag, h := range handlers {
			if containsFragment(sql, frag) {
				return h()
			}
		}
		return &mockRows{}, nil
	}
}

// countRowFn returns a queryRowFn that scans sequential counts for every call.
func countRowFn() func(ctx context.Context, sql string, args ...any) pgx.Row {
	return func(_ context.Context, _ string, _ ...any) pgx.Row {
		return &mockRow{scanFn: countScanFn()}
	}
}

func TestRun_L1_MissingEvidence(t *testing.T) {
	missingID := uuid.New().String()
	var execSQLs []string
	db := &mockDB{
		queryRowFn: countRowFn(),
		queryFn: queryRowsByFragment(map[string]func() (pgx.Rows, error){
			"LEFT JOIN provider_evidence pe ON pe.usage_log_id = ul.id": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{missingID}}}, nil
			},
		}),
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if !containsSQL(execSQLs, "INSERT INTO reconciliation_diffs") {
		t.Fatal("missing_evidence diff not written")
	}
	// createDiff is called with severity 'warning' and diff_type 'missing_evidence'.
	if !containsSQL(execSQLs, "status = 'completed'") {
		t.Error("completeRun UPDATE not executed")
	}
}

func TestRun_L1_UsageMismatch(t *testing.T) {
	mismatchID := uuid.New().String()
	var execSQLs []string
	db := &mockDB{
		queryRowFn: countRowFn(),
		queryFn: queryRowsByFragment(map[string]func() (pgx.Rows, error){
			"COALESCE((ul.usage_raw->>'total_tokens')::int, 0)": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{mismatchID, 350, 342}}}, nil
			},
		}),
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if !containsSQL(execSQLs, "INSERT INTO reconciliation_diffs") {
		t.Fatal("usage_mismatch diff not written")
	}
}

func TestRun_L1_ErrorMislabel(t *testing.T) {
	mislabelID := uuid.New().String()
	var execSQLs []string
	db := &mockDB{
		queryRowFn: countRowFn(),
		queryFn: queryRowsByFragment(map[string]func() (pgx.Rows, error){
			"pe.status_code >= 400": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{mislabelID, 500}}}, nil
			},
		}),
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if !containsSQL(execSQLs, "INSERT INTO reconciliation_diffs") {
		t.Fatal("error_mislabel diff not written")
	}
}

func TestRun_L1_FinderError_StillCompletes(t *testing.T) {
	var execSQLs []string
	db := &mockDB{
		queryRowFn: countRowFn(),
		queryFn: queryRowsByFragment(map[string]func() (pgx.Rows, error){
			"LEFT JOIN provider_evidence pe ON pe.usage_log_id = ul.id": func() (pgx.Rows, error) {
				return nil, errors.New("missing evidence query failed")
			},
		}),
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: L1 finder error should not fail the run, got: %v", err)
	}

	if !containsSQL(execSQLs, "status = 'completed'") {
		t.Error("completeRun UPDATE not executed after L1 finder failure")
	}
}

func TestRun_L1_DiffWriteError_FailsRun(t *testing.T) {
	missingID := uuid.New().String()
	db := &mockDB{
		queryRowFn: countRowFn(),
		queryFn: queryRowsByFragment(map[string]func() (pgx.Rows, error){
			"LEFT JOIN provider_evidence pe ON pe.usage_log_id = ul.id": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{missingID}}}, nil
			},
		}),
		execFn: func(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
			if containsFragment(sql, "INSERT INTO reconciliation_diffs") {
				return pgconn.CommandTag{}, errors.New("diff insert failed")
			}
			return pgconn.CommandTag{}, nil
		},
	}
	r := &Reconciler{pool: db}

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when L1 diff write fails")
	}
}

func TestRun_CombinedL0L1Diffs(t *testing.T) {
	orphanID := uuid.New().String()
	missingID := uuid.New().String()
	mismatchID := uuid.New().String()
	mislabelID := uuid.New().String()

	var execSQLs []string
	db := &mockDB{
		queryRowFn: countRowFn(),
		queryFn: queryRowsByFragment(map[string]func() (pgx.Rows, error){
			// findOrphaned (missing_charge_lines)
			"cl.id IS NULL AND ul.status = 'completed'": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{orphanID}}}, nil
			},
			// findMissingEvidence
			"LEFT JOIN provider_evidence pe ON pe.usage_log_id = ul.id": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{missingID}}}, nil
			},
			// findUsageMismatch
			"COALESCE((ul.usage_raw->>'total_tokens')::int, 0)": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{mismatchID, 100, 90}}}, nil
			},
			// findErrorMislabel
			"pe.status_code >= 400": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{mislabelID, 500}}}, nil
			},
		}),
		execFn: collectExec(&execSQLs),
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	// 4 categories, each with 1 diff => 4 diff inserts.
	diffInserts := 0
	for _, s := range execSQLs {
		if containsFragment(s, "INSERT INTO reconciliation_diffs") {
			diffInserts++
		}
	}
	if diffInserts != 4 {
		t.Errorf("expected 4 diff inserts, got %d", diffInserts)
	}
}

// TestRun_L3_BillingDiffs verifies the L3 pass against a real database: seeded
// billing_records and usage_logs must produce billing_amount_mismatch,
// billing_without_usage, and usage_without_billing diffs.
func TestRun_L3_BillingDiffs(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	keyID := uuid.New()
	connectorID := uuid.New()
	now := time.Now().UTC()
	periodStart := now.Add(-30 * time.Minute)
	periodEnd := now.Add(-5 * time.Minute)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed %s: %v", sql[:40], err)
		}
	}
	mustExec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'x')`,
		userID, "l3@test.local")
	mustExec(`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key) VALUES ($1, $2, 'sk-', 'hash-l3', 'sk-***')`,
		keyID, userID)
	mustExec(`INSERT INTO billing_connectors (id, name, type, base_url) VALUES ($1, 'aliyun-test', 'aliyun', 'https://billing.aliyuncs.com')`,
		connectorID)

	seedUsage := func(id uuid.UUID, requestID, providerReqID string, upstreamCost string, at time.Time) {
		mustExec(`INSERT INTO usage_logs (
			id, user_id, api_key_id, request_id, request_type, public_model_code,
			provider_request_id, usage_source, list_cost, final_cost, upstream_cost,
			currency, status, created_at
		) VALUES ($1,$2,$3,$4,'chat','deepseek-chat',$5,'upstream',0,0,$6,'CNY','completed',$7)`,
			id, userID, keyID, requestID, providerReqID, upstreamCost, at)
	}
	seedBilling := func(id uuid.UUID, externalID, netAmount, externalReqID string, at time.Time) {
		mustExec(`INSERT INTO billing_records (
			id, connector_id, external_id, net_amount, usage_quantity, usage_start_at,
			usage_end_at, external_request_id, created_at
		) VALUES ($1,$2,$3,$4,100,$5,$5,$6,$5)`,
			id, connectorID, externalID, netAmount, at, externalReqID)
	}

	usage1 := uuid.New()
	seedUsage(usage1, "req-1", "req-1", "0.010", periodStart)
	seedBilling(uuid.New(), "billing-1", "0.030", "req-1", periodStart) // mismatch 0.02 > 0.01
	seedBilling(uuid.New(), "billing-2", "0.030", "req-9", periodStart) // no usage
	seedUsage(uuid.New(), "req-2", "req-2", "0.005", periodStart)       // no billing

	r := New(pool)
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	diffCounts := map[string]int{}
	rows, err := pool.Query(ctx,
		`SELECT diff_type, COUNT(*) FROM reconciliation_diffs
		 WHERE run_id = (SELECT id FROM reconciliation_runs ORDER BY created_at DESC LIMIT 1)
		 GROUP BY diff_type`)
	if err != nil {
		t.Fatalf("query diffs: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var diffType string
		var n int
		if err := rows.Scan(&diffType, &n); err != nil {
			t.Fatalf("scan diff: %v", err)
		}
		diffCounts[diffType] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}

	if diffCounts["billing_amount_mismatch"] != 1 {
		t.Errorf("billing_amount_mismatch = %d, want 1", diffCounts["billing_amount_mismatch"])
	}
	if diffCounts["billing_without_usage"] != 1 {
		t.Errorf("billing_without_usage = %d, want 1", diffCounts["billing_without_usage"])
	}
	if diffCounts["usage_without_billing"] != 1 {
		t.Errorf("usage_without_billing = %d, want 1", diffCounts["usage_without_billing"])
	}
	_ = periodEnd
}

func TestRunL2_InternalCrossCheck(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	keyID := uuid.New()
	now := time.Now().UTC()
	periodStart := now.Add(-30 * time.Minute)
	periodEnd := now.Add(-5 * time.Minute)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed %s: %v", sql[:40], err)
		}
	}
	mustExec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'x')`, userID, "l2@test.local")
	mustExec(`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key) VALUES ($1, $2, 'sk-', 'hash-l2', 'sk-***')`,
		keyID, userID)

	seedUsage := func(id uuid.UUID, status string, at time.Time) {
		mustExec(`INSERT INTO usage_logs (
			id, user_id, api_key_id, request_id, request_type, public_model_code,
			usage_source, list_cost, final_cost, currency, status, created_at
		) VALUES ($1,$2,$3,$4,'chat','deepseek-chat','upstream',0,0,'CNY',$5,$6)`,
			id, userID, keyID, "req-"+id.String()[:8], status, at)
	}
	withCharge := func(usageID uuid.UUID) {
		mustExec(`INSERT INTO charge_lines (id, usage_log_id, dimension, unit_name, quantity, unit_price, line_cost)
			VALUES ($1, $2, 'input', '1M tokens', 1, 1, 0.000001)`, uuid.New(), usageID)
	}
	withEvidence := func(usageID uuid.UUID) {
		mustExec(`INSERT INTO provider_evidence (id, usage_log_id, provider, provider_request_id, status_code)
			VALUES ($1, $2, 'deepseek', $3, 200)`, uuid.New(), usageID, "ev-"+usageID.String())
	}

	both := uuid.New()
	seedUsage(both, "completed", periodStart)
	withCharge(both)
	withEvidence(both)

	chargeOnly := uuid.New()
	seedUsage(chargeOnly, "completed", periodStart)
	withCharge(chargeOnly)

	neither := uuid.New()
	seedUsage(neither, "completed", periodStart)

	// Failed usage must be excluded from the L2 snapshot.
	seedUsage(uuid.New(), "failed", periodStart)

	r := &Reconciler{pool: pool}
	q, err := r.runL2(ctx, periodStart, periodEnd)
	if err != nil {
		t.Fatalf("runL2: %v", err)
	}
	if q.UsageLogs != 3 {
		t.Errorf("usage_logs = %d, want 3", q.UsageLogs)
	}
	if q.WithCharge != 2 {
		t.Errorf("with_charge = %d, want 2", q.WithCharge)
	}
	if q.WithEvidence != 1 {
		t.Errorf("with_evidence = %d, want 1", q.WithEvidence)
	}
	if q.BothMissing != 1 {
		t.Errorf("both_missing = %d, want 1", q.BothMissing)
	}
}

// TH-P05-02 (B5 Settle Fallback Visibility Correction).
//
// AC-03: reconciliation must turn an undercharged usage_log flag into a
// review diff (never a wallet mutation). The reconciler holds no wallet
// reference; this test additionally asserts no wallet-touching SQL is
// executed during the whole run.
func TestRun_UnderchargedEvidence_CreatesReviewDiff(t *testing.T) {
	usageLogID := uuid.New().String()
	var execSQLs []string
	var diffArgs [][]any
	db := &mockDB{
		queryRowFn: countRowFn(),
		queryFn: queryRowsByFragment(map[string]func() (pgx.Rows, error){
			"ul.error_code = 'undercharged'": func() (pgx.Rows, error) {
				return &mockRows{items: [][]any{{usageLogID, "12.000000", "8.000000"}}}, nil
			},
		}),
		execFn: func(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
			execSQLs = append(execSQLs, sql)
			if containsFragment(sql, "INSERT INTO reconciliation_diffs") {
				diffArgs = append(diffArgs, args)
			}
			return pgconn.CommandTag{}, nil
		},
	}
	r := &Reconciler{pool: db}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	if len(diffArgs) != 1 {
		t.Fatalf("diff inserts = %d, want exactly 1 (the undercharge review diff)", len(diffArgs))
	}
	args := diffArgs[0]
	if len(args) < 6 {
		t.Fatalf("diff insert args = %d, want 6", len(args))
	}
	if got, _ := args[2].(uuid.UUID); got.String() != usageLogID {
		t.Errorf("usage_log_id = %v, want %s", args[2], usageLogID)
	}
	if diffType, _ := args[3].(string); diffType != "undercharge_review" {
		t.Errorf("diff_type = %q, want undercharge_review", diffType)
	}
	if severity, _ := args[4].(string); severity != "critical" {
		t.Errorf("severity = %q, want critical", severity)
	}
	detail, _ := args[5].(json.RawMessage)
	if !containsFragment(string(detail), `"list_cost":"12.000000"`) ||
		!containsFragment(string(detail), `"wallet_charged":"8.000000"`) {
		t.Errorf("diff_detail = %s, want list_cost/wallet_charged amounts", string(detail))
	}

	// Structural money-safety: a reconciliation run must never execute SQL
	// that mutates wallets or the wallet ledger.
	for _, frag := range []string{"UPDATE wallets", "INSERT INTO wallets", "wallet_transactions"} {
		if containsSQL(execSQLs, frag) {
			t.Errorf("reconciler executed wallet-touching SQL fragment %q", frag)
		}
	}
}

// TestRun_UnderchargedEvidence_RealDB verifies the undercharge review diff
// against a real database: a usage_log flagged error_code='undercharged'
// within the period must produce exactly one undercharge_review diff.
func TestRun_UnderchargedEvidence_RealDB(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	keyID := uuid.New()
	now := time.Now().UTC()
	periodStart := now.Add(-30 * time.Minute)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed %s: %v", sql[:40], err)
		}
	}
	mustExec(`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'x')`,
		userID, "p0502@test.local")
	mustExec(`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key) VALUES ($1, $2, 'sk-', 'hash-p0502', 'sk-***')`,
		keyID, userID)
	mustExec(`INSERT INTO usage_logs (
		id, user_id, api_key_id, request_id, request_type, public_model_code,
		usage_source, list_cost, final_cost, currency, status,
		wallet_charged, error_code, error_message, created_at
	) VALUES ($1,$2,$3,'req-p0502','chat','gpt-4o','upstream',12,12,'CNY','completed',
		8,'undercharged','wallet underfunded',$4)`,
		uuid.New(), userID, keyID, periodStart)

	r := New(pool)
	if err := r.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reconciliation_diffs
		 WHERE diff_type = 'undercharge_review' AND severity = 'critical'
		   AND run_id = (SELECT id FROM reconciliation_runs ORDER BY created_at DESC LIMIT 1)`).Scan(&n); err != nil {
		t.Fatalf("count diffs: %v", err)
	}
	if n != 1 {
		t.Errorf("undercharge_review diffs = %d, want 1", n)
	}
}

// Compile-time interface conformance checks for test mocks.
var (
	_ dbPool   = (*mockDB)(nil)
	_ pgx.Rows = (*mockRows)(nil)
	_ pgx.Row  = (*mockRow)(nil)
)

// TestCreateDiff_CriticalCounter TH-P05-05: critical differences increment
// reconciliation_critical_diffs_total exactly once per persisted diff;
// warning diffs and failed inserts never increment it. This is the metric
// behind TokenHubCriticalReconciliationDiff; counting is strictly
// detection-only and changes nothing about diff persistence.
func TestCreateDiff_CriticalCounter(t *testing.T) {
	count := func() float64 {
		return promtestutil.ToFloat64(metrics.Default.ReconciliationCriticalDiffTotal)
	}
	okDB := &mockDB{}
	failDB := &mockDB{execFn: func(context.Context, string, ...any) (pgconn.CommandTag, error) {
		return pgconn.CommandTag{}, errors.New("insert failed")
	}}
	r := &Reconciler{pool: okDB}
	runID := uuid.New()

	// undercharge_review is critical (TH-P05-02 shape).
	base := count()
	if err := r.createDiff(context.Background(), runID, uuid.New().String(), "undercharge_review", "critical", `{"x":1}`); err != nil {
		t.Fatalf("createDiff critical: %v", err)
	}
	if got := count() - base; got != 1 {
		t.Errorf("critical diff counter delta = %v, want 1", got)
	}

	// warning diffs (e.g. idempotent replay missing charge lines) must NOT
	// increment the critical counter — otherwise known-explained shapes
	// would create permanent alert noise.
	base = count()
	if err := r.createDiff(context.Background(), runID, uuid.New().String(), "missing_charge_lines", "warning", "replay"); err != nil {
		t.Fatalf("createDiff warning: %v", err)
	}
	if got := count() - base; got != 0 {
		t.Errorf("warning diff moved the critical counter by %v, want 0", got)
	}

	// Billing-side choke point counts too (error_mislabel is critical).
	base = count()
	if err := r.createBillingDiff(context.Background(), runID, "error_mislabel", "critical", "mislabel"); err != nil {
		t.Fatalf("createBillingDiff critical: %v", err)
	}
	if got := count() - base; got != 1 {
		t.Errorf("billing critical diff counter delta = %v, want 1", got)
	}

	// Failed persistence is not counted (no row, no alert; the worker
	// cycle failure is visible through worker_cycles_total instead).
	rFail := &Reconciler{pool: failDB}
	base = count()
	if err := rFail.createDiff(context.Background(), runID, uuid.New().String(), "undercharge_review", "critical", "{}"); err == nil {
		t.Fatal("expected insert error")
	}
	if got := count() - base; got != 0 {
		t.Errorf("failed insert moved the critical counter by %v, want 0", got)
	}
}

package reconciliation

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// Compile-time interface conformance checks for test mocks.
var (
	_ dbPool   = (*mockDB)(nil)
	_ pgx.Rows = (*mockRows)(nil)
	_ pgx.Row  = (*mockRow)(nil)
)

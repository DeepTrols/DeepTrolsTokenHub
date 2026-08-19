package testutil

import (
	"context"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/deeptrols/api/internal/pkg/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var schemaNameRE = regexp.MustCompile(`^t_[a-z0-9_]+_[0-9a-f]{8}$`)

func TestPkgKeyFromFile(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{`G:\workspace\demo\DeepTrolsTokenHub\internal\handler\console\ledger_test.go`, "internal_handler_console"},
		{"G:/workspace/demo/DeepTrolsTokenHub/internal/app/bootstrap_test.go", "internal_app"},
		{`G:\workspace\demo\DeepTrolsTokenHub\cmd\api\main_test.go`, "cmd_api"},
		{`C:\somewhere\internal\repository\apikey\postgres_test.go`, "internal_repository_apikey"},
		{"/home/u/repo/internal/repository/testutil/db_test.go", "internal_repository_testutil"},
	}
	for _, tt := range tests {
		if got := pkgKeyFromFile(tt.file); got != tt.want {
			t.Errorf("pkgKeyFromFile(%q) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

func TestPkgKeyFromFile_Unrecognized(t *testing.T) {
	if got := pkgKeyFromFile(`C:\repo\main_test.go`); got != "" {
		t.Errorf("expected empty key for file outside internal/cmd, got %q", got)
	}
}

func TestSchemaNameForKey(t *testing.T) {
	key := "internal_handler_console"
	first, err := schemaNameForKey(key)
	if err != nil {
		t.Fatalf("schemaNameForKey: %v", err)
	}
	second, err := schemaNameForKey(key)
	if err != nil {
		t.Fatalf("schemaNameForKey second call: %v", err)
	}
	if first == second {
		t.Fatalf("expected distinct schema names per call, got %q twice", first)
	}
	for _, name := range []string{first, second} {
		if !schemaNameRE.MatchString(name) {
			t.Errorf("schema name %q does not match %v", name, schemaNameRE)
		}
		if len(name) > 63 {
			t.Errorf("schema name %q exceeds 63 bytes", name)
		}
	}
}

func TestSchemaNameForKey_RejectsInvalidKey(t *testing.T) {
	for _, key := range []string{"", "UPPER", "has-dash", "internal/with/slash"} {
		if _, err := schemaNameForKey(key); err == nil {
			t.Errorf("schemaNameForKey(%q) expected error", key)
		}
	}
}

func TestValidSchemaName(t *testing.T) {
	valid := []string{"t_internal_app_1a2b3c4d", "t_a_00000000"}
	invalid := []string{"public", "pg_catalog", "t_X_1a2b3c4d", "t_internal_app_zzzzzzzz", "t_internal_app_1a2b3c4"}
	for _, name := range valid {
		if !validSchemaName(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range invalid {
		if validSchemaName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestValidateTestDSN(t *testing.T) {
	if err := validateTestDSN("postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols_test?sslmode=disable"); err != nil {
		t.Errorf("expected _test URL to pass, got %v", err)
	}
	if err := validateTestDSN("host=localhost dbname=deeptrols_test"); err != nil {
		t.Errorf("expected keyword-form _test DSN to pass, got %v", err)
	}
	if err := validateTestDSN("postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols?sslmode=disable"); err == nil {
		t.Error("expected non-_test database to fail")
	}
	if err := validateTestDSN("not a dsn at all"); err == nil {
		t.Error("expected malformed DSN to fail")
	}
}

func TestSchemaDSN_BuildsOptionsParam(t *testing.T) {
	base := "postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols_test?sslmode=disable"
	got, err := schemaDSN(base, "t_internal_app_1a2b3c4d")
	if err != nil {
		t.Fatalf("schemaDSN: %v", err)
	}
	cfg, err := pgxpool.ParseConfig(got)
	if err != nil {
		t.Fatalf("parse built DSN: %v", err)
	}
	wantOptions := "-c search_path=t_internal_app_1a2b3c4d,public"
	if gotOpts := cfg.ConnConfig.RuntimeParams["options"]; gotOpts != wantOptions {
		t.Errorf("options = %q, want %q (full DSN %q)", gotOpts, wantOptions, got)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Errorf("database changed to %q", cfg.ConnConfig.Database)
	}
}

func TestSchemaDSN_RejectsKeywordForm(t *testing.T) {
	if _, err := schemaDSN("host=localhost dbname=deeptrols_test", "t_x_1a2b3c4d"); err == nil {
		t.Error("expected keyword-form DSN to be rejected")
	}
}

// TestSetupPool_IsolatedAndMigrated is an integration test: it requires
// TEST_DATABASE_URL and verifies that SetupPool targets a private schema with
// all migrations applied.
func TestSetupPool_IsolatedAndMigrated(t *testing.T) {
	pool := SetupPool(t)
	ctx := context.Background()

	var schema string
	if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&schema); err != nil {
		t.Fatalf("current_schema: %v", err)
	}
	if !schemaNameRE.MatchString(schema) {
		t.Fatalf("current_schema = %q, want harness schema matching %v", schema, schemaNameRE)
	}

	// Tables from early and late migrations must exist in the private schema.
	for _, table := range []string{"users", "tenant_memberships", "provider_evidence", "reconciliation_runs"} {
		var tableSchema string
		err := pool.QueryRow(ctx, `
			SELECT n.nspname
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relname = $1 AND c.relkind = 'r' AND n.nspname = current_schema()`, table).Scan(&tableSchema)
		if err == pgx.ErrNoRows {
			t.Errorf("table %s missing from isolated schema", table)
			continue
		}
		if err != nil {
			t.Fatalf("locate table %s: %v", table, err)
		}
		if tableSchema != schema {
			t.Errorf("table %s lives in schema %s, want %s", table, tableSchema, schema)
		}
	}

	// 000009: usage_source CHECK must allow 'cached'.
	var constraintDef string
	if err := pool.QueryRow(ctx,
		`SELECT pg_get_constraintdef(c.oid)
		 FROM pg_constraint c
		 JOIN pg_class t ON t.oid = c.conrelid
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 WHERE c.conname = 'usage_logs_usage_source_check' AND n.nspname = current_schema()`).
		Scan(&constraintDef); err != nil {
		t.Fatalf("usage_source check lookup in %s: %v", schema, err)
	}
	if !strings.Contains(constraintDef, "'cached'") {
		t.Errorf("usage_source CHECK does not allow cached: %s", constraintDef)
	}

	// 000008: quota_allocations uniqueness constraint must exist.
	var constraintCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*)
		 FROM pg_constraint c
		 JOIN pg_class t ON t.oid = c.conrelid
		 JOIN pg_namespace n ON n.oid = t.relnamespace
		 WHERE c.conname = 'quota_allocations_pool_user_unique' AND n.nspname = current_schema()`).
		Scan(&constraintCount); err != nil {
		t.Fatalf("constraint lookup: %v", err)
	}
	if constraintCount != 1 {
		t.Errorf("quota_allocations_pool_user_unique count = %d, want 1", constraintCount)
	}
}

// TestSetupPool_SameSchemaAcrossCalls verifies a package reuses its schema
// across multiple SetupPool calls (once-per-process provisioning).
func TestSetupPool_SameSchemaAcrossCalls(t *testing.T) {
	first := SetupPool(t)
	second := SetupPool(t)
	ctx := context.Background()

	// The package pool is created once per process and reused, so both calls
	// must return the very same pool. A regression to per-call pools would
	// break the schema-sharing invariant AND leak a connection per test.
	if first != second {
		t.Errorf("SetupPool returned different pools across calls: %p vs %p", first, second)
	}

	var a, b string
	if err := first.QueryRow(ctx, "SELECT current_schema()").Scan(&a); err != nil {
		t.Fatalf("current_schema (first): %v", err)
	}
	if err := second.QueryRow(ctx, "SELECT current_schema()").Scan(&b); err != nil {
		t.Fatalf("current_schema (second): %v", err)
	}
	if a != b {
		t.Errorf("SetupPool used different schemas across calls: %q vs %q", a, b)
	}
}

// TestProvisionSchema_FreshSchema exercises the migration runner on a brand-new
// schema and verifies tables land there.
func TestProvisionSchema_FreshSchema(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := validateTestDSN(dsn); err != nil {
		t.Fatalf("TEST_DATABASE_URL invalid: %v", err)
	}

	schema, err := schemaNameForKey("testutil_fresh")
	if err != nil {
		t.Fatalf("schemaNameForKey: %v", err)
	}
	ctx := context.Background()
	if err := provisionSchema(ctx, dsn, schema); err != nil {
		t.Fatalf("provisionSchema: %v", err)
	}
	t.Cleanup(func() { dropSchema(t, dsn, schema) })

	poolDSN, err := schemaDSN(dsn, schema)
	if err != nil {
		t.Fatalf("schemaDSN: %v", err)
	}
	pool, err := db.NewPool(ctx, poolDSN)
	if err != nil {
		t.Fatalf("NewPool on provisioned schema: %v", err)
	}
	defer pool.Close()

	var tableSchema string
	err = pool.QueryRow(ctx, `
		SELECT n.nspname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = 'usage_logs' AND c.relkind = 'r' AND n.nspname = current_schema()`).Scan(&tableSchema)
	if err != nil {
		t.Fatalf("locate usage_logs: %v", err)
	}
	if tableSchema != schema {
		t.Errorf("usage_logs lives in schema %s, want %s", tableSchema, schema)
	}
}

// TestProvisionSchema_ConcurrentDifferentSchemas provisions two schemas from
// the same process at the same time. The extension lock must serialize the
// database-global CREATE EXTENSION work while both schemas are migrated.
func TestProvisionSchema_ConcurrentDifferentSchemas(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := validateTestDSN(dsn); err != nil {
		t.Fatalf("TEST_DATABASE_URL invalid: %v", err)
	}

	var (
		wg      sync.WaitGroup
		errs    = make(chan error, 2)
		schemas []string
	)
	for _, key := range []string{"testutil_conc_a", "testutil_conc_b"} {
		var err error
		schema, err := schemaNameForKey(key)
		if err != nil {
			t.Fatalf("schemaNameForKey(%q): %v", key, err)
		}
		schemas = append(schemas, schema)
		provisioned := schema
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- provisionSchema(context.Background(), dsn, provisioned)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent provision: %v", err)
		}
	}
	for _, s := range schemas {
		t.Cleanup(func() { dropSchema(t, dsn, s) })
	}
}

// dropSchema removes a harness-created schema. It is only ever called with a
// name that passed validSchemaName and a *_test DSN.
func dropSchema(t *testing.T, dsn, schema string) {
	t.Helper()
	if !validSchemaName(schema) {
		t.Fatalf("refusing to drop invalid schema name %q", schema)
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("dropSchema connect: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
		t.Fatalf("drop schema %s: %v", schema, err)
	}
}

package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SetupPool returns the calling package's shared, schema-isolated connection
// pool. The private schema is created and fully migrated on first use, and the
// pool is reused by every test in the package (it lives for the test binary).
// It skips the test when TEST_DATABASE_URL is unset. It deliberately does NOT
// fall back to DATABASE_URL or to a hardcoded default: the tests TRUNCATE
// tables, and must never run against a database that could hold real
// configuration or business data.
func SetupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping repository integration test")
		return nil
	}
	assertTestDatabase(t, dsn)
	st := stateForPackage(t)
	return st.ensurePool(t, dsn)
}

// assertTestDatabase fails the test when the DSN does not point at a database
// whose name ends with "_test", so a misconfigured environment can never let
// repository tests TRUNCATE a data-bearing database.
func assertTestDatabase(t *testing.T, dsn string) {
	t.Helper()
	if err := validateTestDSN(dsn); err != nil {
		t.Fatal(err)
	}
}

// validateTestDSN fails when the DSN does not point at a database whose name
// ends with "_test", so a misconfigured environment can never let repository
// tests TRUNCATE a data-bearing database.
func validateTestDSN(dsn string) error {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse TEST_DATABASE_URL: %w", err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		return fmt.Errorf("TEST_DATABASE_URL must point at a *_test database, got %q", cfg.ConnConfig.Database)
	}
	return nil
}

// assertPoolIsTestDatabase guards TRUNCATE itself as a second line of
// defense: even if a pool was created outside SetupPool, truncation only
// proceeds against a *_test database.
func assertPoolIsTestDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	dbName := pool.Config().ConnConfig.Database
	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("refusing to TRUNCATE non-test database %q", dbName)
	}
}

// TruncateTables removes all rows from the named tables using TRUNCATE CASCADE.
// No FK ordering needed — CASCADE follows all references. All tables are
// truncated in a single statement: doing one round-trip per table makes every
// integration test pay ~25x the DB latency, which dominates the console suite.
func TruncateTables(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	assertPoolIsTestDatabase(t, pool)
	if len(tables) == 0 {
		return
	}
	for _, table := range tables {
		if !validTableName(table) {
			t.Fatalf("refusing to TRUNCATE unsafe table name %q", table)
		}
	}
	ctx := context.Background()
	stmt := "TRUNCATE TABLE " + strings.Join(tables, ", ") + " CASCADE"
	if _, err := pool.Exec(ctx, stmt); err != nil {
		t.Fatalf("failed to truncate tables: %v", err)
	}
}

// validTableName rejects anything but a bare lowercase identifier. Table names
// in tests are hardcoded, but TRUNCATE is destructive enough that the
// statement builder refuses anything unusual as defense in depth.
func validTableName(name string) bool {
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return len(name) > 0
}

// TruncateAll removes rows from all tables in FK-safe order.
func TruncateAll(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	TruncateTables(t, pool,
		"tenant_invitations",
		"tenant_memberships",
		"reconciliation_diffs",
		"reconciliation_runs",
		"audit_logs",
		"outbox_events",
		"provider_evidence",
		"charge_lines",
		"usage_logs",
		"quota_ledger",
		"quota_allocations",
		"quota_pools",
		"wallet_transactions",
		"wallets",
		"route_policies",
		"channel_instances",
		"channels",
		"tenant_models",
		"model_pricing",
		"models",
		"tenants",
		"api_key_spend",
		"api_keys",
		"login_history",
		"users",
	)
}

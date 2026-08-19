package testutil

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/pkg/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetupPool creates a connection pool from TEST_DATABASE_URL and skips the
// test when it is unset. It deliberately does NOT fall back to DATABASE_URL
// or to a hardcoded default: the tests TRUNCATE tables, and must never run
// against a database that could hold real configuration or business data.
func SetupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping repository integration test")
		return nil
	}
	assertTestDatabase(t, dsn)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("DATABASE_URL not available: %v", err)
		return nil
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// assertTestDatabase fails the test when the DSN does not point at a database
// whose name ends with "_test", so a misconfigured environment can never let
// repository tests TRUNCATE a data-bearing database.
func assertTestDatabase(t *testing.T, dsn string) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatalf("TEST_DATABASE_URL must point at a *_test database, got %q", cfg.ConnConfig.Database)
	}
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
// No FK ordering needed — CASCADE follows all references.
func TruncateTables(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
	assertPoolIsTestDatabase(t, pool)
	ctx := context.Background()
	for _, table := range tables {
		if _, err := pool.Exec(ctx, "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			t.Fatalf("failed to truncate %s: %v", table, err)
		}
	}
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

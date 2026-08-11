package testutil

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/pkg/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SetupPool creates a connection pool from TEST_DATABASE_URL or skips the test.
// Uses a separate test database to avoid truncating production data.
func SetupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		dsn = "postgresql://deeptrols:deeptrols_dev@127.0.0.1:5432/deeptrols_test?sslmode=disable"
	}

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

// TruncateTables removes all rows from the named tables using TRUNCATE CASCADE.
// No FK ordering needed — CASCADE follows all references.
func TruncateTables(t *testing.T, pool *pgxpool.Pool, tables ...string) {
	t.Helper()
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

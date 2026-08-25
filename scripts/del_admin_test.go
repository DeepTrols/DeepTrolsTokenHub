//go:build ignore

// One-off dev script: dry-runs admin-row cleanup inside a rolled-back
// transaction. Build-ignored so it never participates in `go test ./...`;
// run manually when needed with `go test ./scripts/ -run TestDelAdmin` after
// removing the build tag.
package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDelAdmin verifies the admin-cleanup SQL runs without FK violations.
// It runs inside a transaction that is rolled back so the test never deletes
// real data — this is a script-in-test, not a destructive migration.
func TestDelAdmin(t *testing.T) {
	u := "postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols?sslmode=disable"
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, u)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx) // never persist deletions

	adminID := "00000000-0000-0000-0000-000000000000"

	// Delete in FK-safe order. The dependency chain is deep
	// (provider_evidence/charge_lines -> usage_logs -> api_keys/wallets ->
	// users), so CASCADE-clearing the leaf tables first, then deleting the
	// admin user's own rows, avoids foreign-key violations. The whole run is
	// inside a rolled-back transaction so no data is actually removed.
	if _, err := tx.Exec(ctx,
		`TRUNCATE TABLE provider_evidence, charge_lines, wallet_transactions, channel_instances, channels, tenant_models, model_pricing, models, tenants CASCADE`); err != nil {
		t.Fatalf("truncate leaf tables: %v", err)
	}
	for _, q := range []string{
		"DELETE FROM usage_logs WHERE api_key_id IN (SELECT id FROM api_keys WHERE user_id=$1)",
		"DELETE FROM api_key_spend WHERE api_key_id IN (SELECT id FROM api_keys WHERE user_id=$1)",
		"DELETE FROM audit_logs WHERE actor_id=$1",
		"DELETE FROM login_history WHERE user_id=$1",
		"DELETE FROM api_keys WHERE user_id=$1",
		"DELETE FROM wallets WHERE user_id=$1",
		"DELETE FROM users WHERE id=$1",
	} {
		if _, err := tx.Exec(ctx, q, adminID); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
}

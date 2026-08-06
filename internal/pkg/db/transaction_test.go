package db

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// mustPool returns a connected pool or fails the test.
func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := dbURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := NewPool(ctx, url)
	if err != nil {
		t.Fatalf("failed to create test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// ensureTestTable creates a unique per-test table and registers cleanup.
// Returns the table name. Uses a regular table (not TEMP) so that
// different pool connections can all see it.
func ensureTestTable(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	tableName := fmt.Sprintf("_tx_test_%d", rand.Int63())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id   SERIAL PRIMARY KEY,
			val  TEXT NOT NULL
		)
	`, tableName))
	if err != nil {
		t.Fatalf("failed to create test table %s: %v", tableName, err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName))
	})

	return tableName
}

// TestWithTransaction_Commit commits when the function returns nil.
func TestWithTransaction_Commit(t *testing.T) {
	pool := mustPool(t)
	tbl := ensureTestTable(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (val) VALUES ($1)`, tbl), "commit-test")
		return execErr
	})
	if err != nil {
		t.Fatalf("WithTransaction failed: %v", err)
	}

	// Verify the row is visible outside the transaction (committed).
	var count int
	err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE val = 'commit-test'`, tbl)).Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 row, got %d", count)
	}
}

// TestWithTransaction_Rollback rolls back when the function returns an error.
func TestWithTransaction_Rollback(t *testing.T) {
	pool := mustPool(t)
	tbl := ensureTestTable(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		if _, execErr := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (val) VALUES ($1)`, tbl), "rollback-test"); execErr != nil {
			return execErr
		}
		return pgx.ErrTxClosed // any non-nil error triggers rollback
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify the row was NOT committed.
	var count int
	err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE val = 'rollback-test'`, tbl)).Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", count)
	}
}

// TestWithTransaction_PanicRollback rolls back when the function panics.
func TestWithTransaction_PanicRollback(t *testing.T) {
	pool := mustPool(t)
	tbl := ensureTestTable(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Capture the panic and verify it propagates.
	var panicked bool
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()

		_ = WithTransaction(ctx, pool, func(tx pgx.Tx) error {
			if _, execErr := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (val) VALUES ($1)`, tbl), "panic-test"); execErr != nil {
				return execErr
			}
			panic("deliberate panic to test rollback")
		})
	}()

	if !panicked {
		t.Fatal("expected panic to propagate")
	}

	// Verify the row was NOT committed (rolled back).
	var count int
	err := pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE val = 'panic-test'`, tbl)).Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 rows after panic-rollback, got %d", count)
	}
}

// TestWithTransaction_Nested_Call runs two independent transactions sequentially.
func TestWithTransaction_Nested_Call(t *testing.T) {
	pool := mustPool(t)
	tbl := ensureTestTable(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First transaction commits.
	err := WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (val) VALUES ($1)`, tbl), "first")
		return execErr
	})
	if err != nil {
		t.Fatalf("first tx failed: %v", err)
	}

	// Second transaction commits.
	err = WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (val) VALUES ($1)`, tbl), "second")
		return execErr
	})
	if err != nil {
		t.Fatalf("second tx failed: %v", err)
	}

	var count int
	err = pool.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE val IN ('first', 'second')`, tbl)).Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

// TestWithTransaction_ErrorWrapped verifies that the error returned by fn is propagated.
func TestWithTransaction_ErrorWrapped(t *testing.T) {
	pool := mustPool(t)
	_ = ensureTestTable(t, pool) // create a table, not needed for this test but keeps test inline

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sentinel := &testError{msg: "custom error"}
	err := WithTransaction(ctx, pool, func(tx pgx.Tx) error {
		return sentinel
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err != sentinel {
		t.Fatalf("expected sentinel error, got: %v", err)
	}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

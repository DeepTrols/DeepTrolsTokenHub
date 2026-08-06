package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WithTransaction executes fn inside a database transaction.
// If fn returns nil, the transaction is committed.
// If fn returns an error (or panics), the transaction is rolled back.
// Panics are recovered and re-thrown after rollback.
func WithTransaction(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) (err error) {
	tx, txErr := pool.Begin(ctx)
	if txErr != nil {
		return fmt.Errorf("db: failed to begin transaction: %w", txErr)
	}

	// Ensure rollback on panic or error.
	defer func() {
		if p := recover(); p != nil {
			// Attempt rollback but do not mask the original panic.
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}

	if commitErr := tx.Commit(ctx); commitErr != nil {
		return fmt.Errorf("db: failed to commit transaction: %w", commitErr)
	}

	return nil
}

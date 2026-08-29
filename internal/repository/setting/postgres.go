package setting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository with PostgreSQL via pgx/v5.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// compile-time interface check
var _ Repository = (*PostgresRepository)(nil)

// All returns every system_settings row.
func (r *PostgresRepository) All(ctx context.Context) ([]Entry, error) {
	rows, err := r.pool.Query(ctx, `SELECT key, value FROM system_settings`)
	if err != nil {
		return nil, fmt.Errorf("setting all: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Key, &e.Value); err != nil {
			return nil, fmt.Errorf("setting all scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("setting all rows: %w", err)
	}
	return out, nil
}

// Get returns the rows matching the given keys (any order).
func (r *PostgresRepository) Get(ctx context.Context, keys ...string) ([]Entry, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT key, value FROM system_settings WHERE key = ANY($1)`, keys)
	if err != nil {
		return nil, fmt.Errorf("setting get: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Key, &e.Value); err != nil {
			return nil, fmt.Errorf("setting get scan: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("setting get rows: %w", err)
	}
	return out, nil
}

// Upsert inserts or updates the given entries atomically.
func (r *PostgresRepository) Upsert(ctx context.Context, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("setting upsert begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, e := range entries {
		if _, err := tx.Exec(ctx,
			`INSERT INTO system_settings (key, value, updated_at) VALUES ($1, $2, NOW())
			 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
			e.Key, e.Value); err != nil {
			return fmt.Errorf("setting upsert: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("setting upsert commit: %w", err)
	}
	return nil
}

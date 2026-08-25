package minutebucket

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore is the fallback minute bucket implementation using the
// api_key_quota_buckets table.
type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Reserve(ctx context.Context, keyID string, tokens int64, rpmLimit int, tpmLimit int64, now time.Time) (Result, error) {
	minute := bucketMinute(now)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback(ctx)

	var requests, used int64
	err = tx.QueryRow(ctx,
		`INSERT INTO api_key_quota_buckets (key_id, bucket, requests, tokens)
		 VALUES ($1, $2, 1, $3)
		 ON CONFLICT (key_id, bucket) DO UPDATE SET
		   requests = api_key_quota_buckets.requests + 1,
		   tokens = api_key_quota_buckets.tokens + EXCLUDED.tokens
		 RETURNING requests, tokens`,
		keyID, minute, tokens).Scan(&requests, &used)
	if err != nil {
		return Result{}, fmt.Errorf("minute bucket reserve: %w", err)
	}

	allowed := true
	if rpmLimit > 0 && requests > int64(rpmLimit) {
		allowed = false
	}
	if tpmLimit > 0 && used > tpmLimit {
		allowed = false
	}
	if !allowed {
		// Roll back the reservation.
		if _, err := tx.Exec(ctx,
			`UPDATE api_key_quota_buckets
			 SET requests = GREATEST(requests - 1, 0), tokens = GREATEST(tokens - $3, 0)
			 WHERE key_id = $1 AND bucket = $2`,
			keyID, minute, tokens); err != nil {
			return Result{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Result{}, err
		}
		// Re-read post-rollback counters for the rate-limit headers.
		_ = s.pool.QueryRow(ctx,
			`SELECT requests, tokens FROM api_key_quota_buckets WHERE key_id = $1 AND bucket = $2`,
			keyID, minute).Scan(&requests, &used)
		return Result{Allowed: false, Requests: requests, Tokens: used}, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return Result{Allowed: true, Requests: requests, Tokens: used}, nil
}

func (s *PostgresStore) Settle(ctx context.Context, keyID string, reserved, actual int64, now time.Time) error {
	delta := actual - reserved
	if delta == 0 {
		return nil
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE api_key_quota_buckets SET tokens = GREATEST(tokens + $3, 0)
		 WHERE key_id = $1 AND bucket = $2`,
		keyID, bucketMinute(now), delta)
	return err
}

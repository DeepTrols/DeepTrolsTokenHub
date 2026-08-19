package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// dbURL fetches TEST_DATABASE_URL or skips the test. It deliberately does not
// fall back to DATABASE_URL: these tests touch a live PostgreSQL server and
// must never run against a data-bearing database. The *_test suffix guard is
// duplicated here (instead of reusing repository/testutil) because importing
// testutil from this package would create an import cycle.
func dbURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatalf("TEST_DATABASE_URL must point at a *_test database, got %q", cfg.ConnConfig.Database)
	}
	return url
}

// TestNewPool_ValidURL creates a pool with a valid connection string and verifies ping.
func TestNewPool_ValidURL(t *testing.T) {
	url := dbURL(t)

	// 30s instead of 10s: a full parallel `go test ./...` starts every package
	// provisioning its schema at once, so a Ping can take seconds while the DB
	// is busy with concurrent migrations. Normal ping latency is sub-second.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, url)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	// Verify pool is alive
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping failed: %v", err)
	}

	pool.Close()
}

// TestNewPool_InvalidURL_Malformed returns an error when the connection string is junk.
func TestNewPool_InvalidURL_Malformed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := NewPool(ctx, "not-a-valid-url-!!@#")
	if err == nil {
		t.Fatal("expected error for malformed URL, got nil")
	}
}

// TestNewPool_InvalidURL_Unreachable returns an error when the host does not exist.
func TestNewPool_InvalidURL_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := NewPool(ctx, "postgresql://nonexistent:5432/nodb?sslmode=disable&connect_timeout=2")
	if err == nil {
		t.Fatal("expected error for unreachable host, got nil")
	}
}

// TestNewPool_EmptyURL returns an error for an empty connection string.
func TestNewPool_EmptyURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := NewPool(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

// TestNewPool_Close releases resources and subsequent operations fail gracefully.
func TestNewPool_Close(t *testing.T) {
	url := dbURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, url)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	pool.Close()

	// After close, operations on the pool should return an error.
	err = pool.Ping(ctx)
	if err == nil {
		t.Fatal("expected error when pinging a closed pool")
	}
}

// TestPool_Stats_PostConnect verifies pool statistics are non-zero after a connection.
func TestPool_Stats_PostConnect(t *testing.T) {
	url := dbURL(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, url)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	defer pool.Close()

	stat := pool.Stat()
	if stat == nil {
		t.Fatal("expected non-nil pool stats")
	}
	t.Logf("pool stats: total=%d idle=%d acquired=%d",
		stat.TotalConns(), stat.IdleConns(), stat.AcquiredConns())
}

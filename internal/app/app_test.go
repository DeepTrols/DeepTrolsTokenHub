package app

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/config"
)

// requireDBEnv skips the test if DATABASE_URL is not set.
func requireDBEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
}

// TestNewApp_Success creates an App with a real database pool and verifies health.
func TestNewApp_Success(t *testing.T) {
	requireDBEnv(t)

	cfg := &config.Config{
		DB: config.DBConfig{
			URL: os.Getenv("DATABASE_URL"),
		},
	}

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil app")
	}
	if app.Pool == nil {
		t.Fatal("expected non-nil pool")
	}

	// Verify the pool is healthy.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.Pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping failed: %v", err)
	}

	// Clean up.
	app.Pool.Close()
}

// TestNewApp_InvalidDBURL returns an error for a bad URL.
func TestNewApp_InvalidDBURL(t *testing.T) {
	cfg := &config.Config{
		DB: config.DBConfig{
			URL: "not-a-valid-url::!!",
		},
	}

	_, err := NewApp(cfg)
	if err == nil {
		t.Fatal("expected error for invalid DB URL, got nil")
	}
}

// TestNewApp_UnreachableDB returns an error when host is unreachable.
func TestNewApp_UnreachableDB(t *testing.T) {
	cfg := &config.Config{
		DB: config.DBConfig{
			URL: "postgresql://nonexistent-host:5432/nodb?sslmode=disable&connect_timeout=2",
		},
	}

	_, err := NewApp(cfg)
	if err == nil {
		t.Fatal("expected error for unreachable DB, got nil")
	}
}

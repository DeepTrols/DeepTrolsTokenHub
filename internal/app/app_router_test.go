package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/config"
	"github.com/go-chi/chi/v5"
)

// TestRegisterRoutes_HealthEndpoint verifies that RegisterRoutes
// attaches the /health endpoint that responds with 200.
func TestRegisterRoutes_HealthEndpoint(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	cfg := &config.Config{
		DB: config.DBConfig{URL: url},
	}

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	defer app.Shutdown()

	r := chi.NewRouter()
	app.RegisterRoutes(r)

	// Hit the /health endpoint.
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

// TestShutdown_ClosesPool verifies that Shutdown releases the pool
// and subsequent operations fail gracefully.
func TestShutdown_ClosesPool(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	cfg := &config.Config{
		DB: config.DBConfig{URL: url},
	}

	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}

	app.Shutdown()

	// After shutdown, the pool should be closed and operations should fail.
	if app.Pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		err := app.Pool.Ping(ctx)
		if err == nil {
			t.Fatal("expected error when pinging a closed pool after Shutdown")
		}
	}

	// Calling Shutdown again should not panic.
	app.Shutdown()
}

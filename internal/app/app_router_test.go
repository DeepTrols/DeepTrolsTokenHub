package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/config"
	"github.com/go-chi/chi/v5"
)

// TestRegisterRoutes_HealthEndpoint verifies that RegisterRoutes
// attaches the /health endpoint that responds with 200.
func TestRegisterRoutes_HealthEndpoint(t *testing.T) {
	url := testDBURL()
	if url == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL not set; skipping integration test")
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

// TestRegisterRoutes_LivenessAndReadiness verifies /healthz and /readyz are
// registered; with a live database /readyz must report database ok.
func TestRegisterRoutes_LivenessAndReadiness(t *testing.T) {
	url := testDBURL()
	if url == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL not set; skipping integration test")
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

	liveness := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	lrec := httptest.NewRecorder()
	r.ServeHTTP(lrec, liveness)
	if lrec.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", lrec.Code)
	}

	readiness := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rrec := httptest.NewRecorder()
	r.ServeHTTP(rrec, readiness)
	if rrec.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200; body=%s", rrec.Code, rrec.Body.String())
	}
	if !strings.Contains(rrec.Body.String(), `"database":"ok"`) {
		t.Fatalf("/readyz body missing database ok: %s", rrec.Body.String())
	}
}

// TestShutdown_ClosesPool verifies that Shutdown releases the pool
// and subsequent operations fail gracefully.
func TestShutdown_ClosesPool(t *testing.T) {
	url := testDBURL()
	if url == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL not set; skipping integration test")
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

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/pkg/metrics"
	"github.com/go-chi/chi/v5"
)

// TestRegisterRoutes_HealthEndpoint verifies that RegisterRoutes
// attaches the /health endpoint that responds with 200.
func TestRegisterRoutes_HealthEndpoint(t *testing.T) {
	url := testDBURL(t)

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
	url := testDBURL(t)

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

// TestRegisterRoutes_MetricsEndpoint (TH-P05-04 regression) verifies
// /metrics is mounted by RegisterRoutes, serves the Prometheus text format
// with the correct Content-Type, and exposes the baseline metric families —
// while /health and /readyz behavior is unchanged by the instrumentation.
func TestRegisterRoutes_MetricsEndpoint(t *testing.T) {
	url := testDBURL(t)

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

	// CounterVec families only appear in the exposition once at least one
	// child exists; materialize one request series (delta-safe: every other
	// assertion in the suite is before/after based).
	metrics.RecordRequest("models", http.StatusOK, time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("/metrics Content-Type = %q, want text/plain*", ct)
	}
	body := rec.Body.String()
	for _, family := range []string{"gateway_requests_total", "billing_reserve_total"} {
		if !strings.Contains(body, family) {
			t.Errorf("/metrics body missing baseline family %s", family)
		}
	}

	// Regression: operational endpoints remain intact next to /metrics.
	health := httptest.NewRequest(http.MethodGet, "/health", nil)
	hrec := httptest.NewRecorder()
	r.ServeHTTP(hrec, health)
	if hrec.Code != http.StatusOK {
		t.Fatalf("/health status = %d, want 200", hrec.Code)
	}
	ready := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rrec := httptest.NewRecorder()
	r.ServeHTTP(rrec, ready)
	if rrec.Code != http.StatusOK {
		t.Fatalf("/readyz status = %d, want 200; body=%s", rrec.Code, rrec.Body.String())
	}
}

// TestShutdown_ClosesPool verifies that Shutdown releases the pool
// and subsequent operations fail gracefully.
func TestShutdown_ClosesPool(t *testing.T) {
	url := testDBURL(t)

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

package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHealthHandler returns 200 and a JSON body with status "ok".
func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `"status"`) || !strings.Contains(body, `"ok"`) {
		t.Fatalf("expected status:ok in body, got: %s", body)
	}
}

// TestHealthHandler_HeadMethod verifies other methods also work.
func TestHealthHandler_HeadMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodHead, "/health", nil)
	rec := httptest.NewRecorder()

	healthHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

// TestHealthzHandler_AlwaysOK verifies the liveness probe always answers 200.
func TestHealthzHandler_AlwaysOK(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthzHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("expected status ok, got: %s", rec.Body.String())
	}
}

// TestReadyzHandler_NilDependencies verifies readyz is 200 when no
// dependencies are configured (unit-level, no DB/Redis).
func TestReadyzHandler_NilDependencies(t *testing.T) {
	a := &App{} // nil Pool and nil Redis
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	readyzHandler(a).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ready"`) {
		t.Fatalf("expected ready, got: %s", rec.Body.String())
	}
}

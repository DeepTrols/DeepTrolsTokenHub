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

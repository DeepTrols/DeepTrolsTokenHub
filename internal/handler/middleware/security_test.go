package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSecurityHeaders_SetsAllRequiredHeaders verifies that all five security
// headers are set on responses that pass through the middleware.
func TestSecurityHeaders_SetsAllRequiredHeaders(t *testing.T) {
	// Arrange
	mw := SecurityHeaders()
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	handler := mw(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: all five headers must be present
	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"},
		{"Content-Security-Policy", "default-src 'self'"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := w.Header().Get(tt.header)
			if got != tt.want {
				t.Errorf("header %s = %q, want %q", tt.header, got, tt.want)
			}
		})
	}

	// Assert: response body is still passed through
	if w.Body.String() != "OK" {
		t.Errorf("body = %q, want %q", w.Body.String(), "OK")
	}

	// Assert: status code is still passed through
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// TestSecurityHeaders_HeadersPersistThroughMiddlewareChain verifies that
// security headers survive through a chain of multiple middlewares.
func TestSecurityHeaders_HeadersPersistThroughMiddlewareChain(t *testing.T) {
	// Arrange: create a middleware chain
	dummyMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Dummy", "present")
			next.ServeHTTP(w, r)
		})
	}

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("chain OK"))
	})

	// Chain: SecurityHeaders -> dummy -> final
	mw := SecurityHeaders()
	handler := mw(dummyMiddleware(finalHandler))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: security headers are still present
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q after chain, want nosniff", got)
	}
	if got := w.Header().Get("X-Dummy"); got != "present" {
		t.Errorf("X-Dummy header was lost: got %q, want present", got)
	}
	if got := w.Header().Get("Strict-Transport-Security"); got == "" {
		t.Error("Strict-Transport-Security header missing after chain")
	}
}

// TestSecurityHeaders_WithErrorStatusCodes verifies headers are also set
// on error responses (non-200).
func TestSecurityHeaders_WithErrorStatusCodes(t *testing.T) {
	// Arrange
	mw := SecurityHeaders()
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	handler := mw(nextHandler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: headers are set even on 404
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q on error, want nosniff", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q on error, want DENY", got)
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// TestSecurityHeaders_IshttpHandlerFunc verifies the middleware returns
// a function that accepts and returns http.Handler.
func TestSecurityHeaders_IsHTTPHandlerFunc(t *testing.T) {
	var fn interface{} = SecurityHeaders()
	if _, ok := fn.(func(http.Handler) http.Handler); !ok {
		t.Error("SecurityHeaders() should return func(http.Handler) http.Handler")
	}
}

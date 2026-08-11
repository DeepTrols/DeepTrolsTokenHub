package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/pkg/ratelimit"
)

// TestLoginRateLimit_FirstFiveRequestsPass verifies that the first 5 requests
// from the same IP pass through to the next handler.
func TestLoginRateLimit_FirstFiveRequestsPass(t *testing.T) {
	// Arrange
	limit := 5
	window := 1 * time.Minute
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := LoginRateLimit(limiter, limit, window)

	callCount := 0
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	ip := "192.168.1.100:12345"

	// Act: send 5 requests from the same IP
	for i := 0; i < limit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Assert: each request should pass through
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}

	if callCount != limit {
		t.Errorf("next handler called %d times, want %d", callCount, limit)
	}
}

// TestLoginRateLimit_SixthRequestReturns429 verifies that the 6th request
// from the same IP returns a 429 Too Many Requests.
func TestLoginRateLimit_SixthRequestReturns429(t *testing.T) {
	// Arrange
	limit := 5
	window := 1 * time.Minute
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := LoginRateLimit(limiter, limit, window)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	ip := "192.168.1.200:12345"

	// Send 5 requests to exhaust the limit
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = ip
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// Act: 6th request
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// Assert: 429 status
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d (429 Too Many Requests)", w.Code, http.StatusTooManyRequests)
	}

	// Assert: error response body
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["error"] != "Too many requests, please try again later" {
		t.Errorf("error = %q, want %q", body["error"], "Too many requests, please try again later")
	}

	// Assert: Retry-After header is present
	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("Retry-After header is missing")
	}
}

// TestLoginRateLimit_DifferentIPNotRateLimited verifies that a different
// IP address is not affected by another IP's rate limit.
func TestLoginRateLimit_DifferentIPNotRateLimited(t *testing.T) {
	// Arrange
	limit := 5
	window := 1 * time.Minute
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := LoginRateLimit(limiter, limit, window)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	ip1 := "192.168.1.10:12345"
	ip2 := "10.0.0.1:54321"

	// Exhaust limit for ip1
	req1 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req1.RemoteAddr = ip1
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req1)
	}

	// Verify ip1 is blocked
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusTooManyRequests {
		t.Errorf("ip1 6th request: status = %d, want %d", w1.Code, http.StatusTooManyRequests)
	}

	// Act: ip2 should still pass through
	req2 := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req2.RemoteAddr = ip2
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	// Assert: ip2 is not rate limited
	if w2.Code != http.StatusOK {
		t.Errorf("ip2 first request: status = %d, want %d", w2.Code, http.StatusOK)
	}
}

// TestLoginRateLimit_CleanupGoroutineExpiresEntries verifies that entries
// are cleaned up after the window passes.
func TestLoginRateLimit_WindowExpires(t *testing.T) {
	// Arrange: use a very short window for testing
	limit := 2
	window := 50 * time.Millisecond
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := LoginRateLimit(limiter, limit, window)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	ip := "192.168.1.50:12345"
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = ip

	// Exhaust the limit
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// Verify blocked
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exhausting limit, got %d", w.Code)
	}

	// Act: wait for the window to expire
	time.Sleep(window + 20*time.Millisecond)

	// Assert: requests should pass again after window expires
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)
	if w2.Code != http.StatusOK {
		t.Errorf("after window expired: status = %d, want %d", w2.Code, http.StatusOK)
	}
}

// TestLoginRateLimit_EmptyRemoteAddrIsTracked verifies that empty RemoteAddr
// is still tracked (all empty IPs share the same bucket).
func TestLoginRateLimit_EmptyRemoteAddrIsTracked(t *testing.T) {
	// Arrange
	limit := 3
	window := 1 * time.Minute
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := LoginRateLimit(limiter, limit, window)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	// All requests with empty RemoteAddr should share the same bucket
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = ""

	// Use up the limit
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// Next should be blocked
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("empty RemoteAddr: status = %d, want %d after limit exhausted", w.Code, http.StatusTooManyRequests)
	}
}

// TestLoginRateLimit_ReturnsFuncHandler verifies the middleware returns
// a function that accepts and returns http.Handler.
func TestLoginRateLimit_ReturnsFuncHandler(t *testing.T) {
	var fn interface{} = LoginRateLimit(ratelimit.NewMemoryRateLimiter(), 5, 1*time.Minute)
	if _, ok := fn.(func(http.Handler) http.Handler); !ok {
		t.Error("LoginRateLimit should return func(http.Handler) http.Handler")
	}
}

// TestTeamRateLimit_UsesUserIDForKey verifies TeamRateLimit keys on the console
// user ID from context, so two users sharing an IP are not rate-limited together.
func TestTeamRateLimit_UsesUserIDForKey(t *testing.T) {
	// Arrange
	limit := 5
	window := 1 * time.Minute
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := TeamRateLimit(limiter, limit, window)

	callCount := 0
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	ip := "192.168.1.99:12345"
	userID := "user-123"

	// Exhaust the limit for user-123
	for i := 0; i < limit; i++ {
		req := httptest.NewRequest(http.MethodPost, "/team/invite", nil)
		req.RemoteAddr = ip
		req = req.WithContext(context.WithValue(req.Context(), CtxUserID, userID))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}

	// user-123 is now blocked
	req := httptest.NewRequest(http.MethodPost, "/team/invite", nil)
	req.RemoteAddr = ip
	req = req.WithContext(context.WithValue(req.Context(), CtxUserID, userID))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("blocked user request: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}

	// A different user on the same IP still passes
	req2 := httptest.NewRequest(http.MethodPost, "/team/invite", nil)
	req2.RemoteAddr = ip
	req2 = req2.WithContext(context.WithValue(req2.Context(), CtxUserID, "user-456"))
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("different user request: status = %d, want %d", w2.Code, http.StatusOK)
	}
}

// TestTeamRateLimit_FallsBackToIP verifies that TeamRateLimit uses the IP when
// no user ID is present in the request context.
func TestTeamRateLimit_FallsBackToIP(t *testing.T) {
	// Arrange
	limit := 2
	window := 1 * time.Minute
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := TeamRateLimit(limiter, limit, window)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	ip := "203.0.113.7:12345"
	req := httptest.NewRequest(http.MethodPost, "/team/invite", nil)
	req.RemoteAddr = ip

	// Exhaust the limit (no user ID in context -> IP key)
	for i := 0; i < limit; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}

	// Next should be blocked
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("IP-fallback request: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

// TestLoginRateLimit_ContentTypeIsJSON verifies the 429 response has
// Content-Type: application/json.
func TestLoginRateLimit_ContentTypeIsJSON(t *testing.T) {
	// Arrange
	limit := 1
	window := 1 * time.Minute
	limiter := ratelimit.NewMemoryRateLimiter()
	mw := LoginRateLimit(limiter, limit, window)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(nextHandler)

	ip := "192.168.1.60:12345"
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	req.RemoteAddr = ip

	// Exhaust the limit
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req)

	// Trigger rate limit
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req)

	// Assert
	ct := w2.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

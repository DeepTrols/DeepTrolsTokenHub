package health_checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubDoer implements doer for testing without a real network.
type stubDoer struct {
	doFn func(req *http.Request) (*http.Response, error)
}

func (s *stubDoer) Do(req *http.Request) (*http.Response, error) {
	if s.doFn != nil {
		return s.doFn(req)
	}
	return &http.Response{StatusCode: http.StatusOK}, nil
}

func TestCheckChannelProbe_HealthyInstance(t *testing.T) {
	// Arrange: a healthy upstream returns 200 OK
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	doer := &stubDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			return http.Get(req.URL.String())
		},
	}

	checker := &Checker{httpClient: doer}

	// Act
	err := checker.probeHealth(context.Background(), upstream.URL)

	// Assert
	if err != nil {
		t.Fatalf("expected no error for healthy instance, got: %v", err)
	}
}

func TestCheckChannelProbe_UnhealthyStatus(t *testing.T) {
	// Arrange: an unhealthy upstream returns 503 Service Unavailable
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	doer := &stubDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			return http.Get(req.URL.String())
		},
	}

	checker := &Checker{httpClient: doer}

	// Act
	err := checker.probeHealth(context.Background(), upstream.URL)

	// Assert
	if err == nil {
		t.Fatal("expected error for unhealthy instance (503), got nil")
	}
}

func TestCheckChannelProbe_401Reachable(t *testing.T) {
	// Arrange: OpenAI-compatible upstreams usually have no /health endpoint and
	// answer 401 (auth required). The server responding at all proves the
	// channel is reachable, so 401 must NOT mark it unhealthy.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	checker := &Checker{httpClient: http.DefaultClient}

	// Act
	err := checker.probeHealth(context.Background(), upstream.URL)

	// Assert
	if err != nil {
		t.Fatalf("expected no error for 401 (reachable upstream), got: %v", err)
	}
}

func TestCheckChannelProbe_ConnectionRefusedError(t *testing.T) {
	// Arrange: a server that is shut down before probe
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	upstreamURL := upstream.URL
	upstream.Close() // close to simulate connection refused

	checker := &Checker{httpClient: http.DefaultClient}

	// Act
	err := checker.probeHealth(context.Background(), upstreamURL)

	// Assert
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}

func TestCheckChannelProbe_Non200SuccessRange(t *testing.T) {
	// Arrange: 201 Created is non-standard but should still be considered healthy
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()

	doer := &stubDoer{
		doFn: func(req *http.Request) (*http.Response, error) {
			return http.Get(req.URL.String())
		},
	}

	checker := &Checker{httpClient: doer}

	// Act: 201 is in the 2xx range, should pass
	err := checker.probeHealth(context.Background(), upstream.URL)

	// Assert
	if err != nil {
		t.Fatalf("expected no error for 2xx status (201), got: %v", err)
	}
}

func TestCheckChannelProbe_TimeoutContext(t *testing.T) {
	// Arrange: a slow upstream that exceeds context deadline
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hang until context cancels
		select {
		case <-r.Context().Done():
			return
		}
	}))
	defer upstream.Close()

	checker := &Checker{httpClient: http.DefaultClient}

	ctx, cancel := context.WithTimeout(context.Background(), 0) // expire immediately
	defer cancel()

	// Act
	err := checker.probeHealth(ctx, upstream.URL)

	// Assert
	if err == nil {
		t.Fatal("expected error for context timeout, got nil")
	}
}

func TestProbeHealth_AppendsHealthPath(t *testing.T) {
	// Arrange: verify that probeHealth appends /health to the base URL
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// If /health is NOT appended, the handler would return the wrong path
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	checker := &Checker{httpClient: http.DefaultClient}

	// Act
	err := checker.probeHealth(context.Background(), upstream.URL)

	// Assert: should succeed because /health was appended and returned 200
	if err != nil {
		t.Fatalf("expected no error when /health returns 200, got: %v", err)
	}
}

func TestCheckerHTTPClientInterface(t *testing.T) {
	// This test verifies the interface compile-time check works.
	// If doer interface is defined but http.Client doesn't match,
	// this test (and the whole package) won't compile.
	var _ doer = http.DefaultClient
	// Just a compile-time assertion — no runtime behavior to check.
}

func TestHealthStatusForScore(t *testing.T) {
	cases := []struct {
		score int
		want  string
	}{
		{100, "healthy"},
		{70, "healthy"},
		{69, "degraded"},
		{30, "degraded"},
		{29, "unhealthy"},
		{0, "unhealthy"},
	}
	for _, c := range cases {
		if got := healthStatusForScore(c.score); got != c.want {
			t.Errorf("healthStatusForScore(%d) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestAdjustHealthScore(t *testing.T) {
	cases := []struct {
		name    string
		current int
		healthy bool
		want    int
	}{
		{"healthy from 0", 0, true, 30},
		{"healthy clamps at 100", 90, true, 100},
		{"healthy already at 100", 100, true, 100},
		{"unhealthy from 100", 100, false, 70},
		{"unhealthy clamps at 0", 20, false, 0},
		{"unhealthy already at 0", 0, false, 0},
		{"degraded stays in band", 40, true, 70},
		{"degraded drops in band", 40, false, 10},
	}
	for _, c := range cases {
		if got := adjustHealthScore(c.current, c.healthy); got != c.want {
			t.Errorf("%s: adjustHealthScore(%d, %v) = %d, want %d", c.name, c.current, c.healthy, got, c.want)
		}
	}
}

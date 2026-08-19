package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func captureRequestLog(t *testing.T, status int) map[string]any {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := RequestLogger(logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.RemoteAddr = "10.1.2.3:5678"
	req.Header.Set("X-Request-ID", "req-abc")
	ctx := context.WithValue(req.Context(), CtxUserID, "user-42")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	mw(next).ServeHTTP(rec, req)

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("request log is not valid JSON: %v; raw=%q", err, buf.String())
	}
	return out
}

func TestRequestLogger_RecordsStructuredFields(t *testing.T) {
	out := captureRequestLog(t, http.StatusOK)

	if out["method"] != "GET" || out["path"] != "/api/console/me" {
		t.Errorf("method/path = %v/%v, want GET /api/console/me", out["method"], out["path"])
	}
	if out["request_id"] != "req-abc" {
		t.Errorf("request_id = %v, want req-abc", out["request_id"])
	}
	if out["user_id"] != "user-42" {
		t.Errorf("user_id = %v, want user-42", out["user_id"])
	}
	if out["client_ip"] != "10.1.2.3" {
		t.Errorf("client_ip = %v, want 10.1.2.3", out["client_ip"])
	}
	if out["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200", out["status"])
	}
	if _, ok := out["duration_ms"].(float64); !ok {
		t.Errorf("duration_ms missing or not numeric: %v", out["duration_ms"])
	}
	if out["level"] != "INFO" {
		t.Errorf("level = %v, want INFO for 2xx", out["level"])
	}
}

func TestRequestLogger_GeneratesRequestIDWhenMissing(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := RequestLogger(logger)
	var ctxID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID, _ = r.Context().Value(CtxRequestID).(string)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	mw(next).ServeHTTP(httptest.NewRecorder(), req)

	if !strings.Contains(buf.String(), `"request_id"`) {
		t.Fatalf("request_id field missing: %s", buf.String())
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON log: %v", err)
	}
	if id, _ := out["request_id"].(string); id == "" {
		t.Errorf("request_id should be generated when the client sends none")
	}
	if ctxID == "" || ctxID != out["request_id"] {
		t.Errorf("CtxRequestID = %q, want %v (downstream must share the logged id)", ctxID, out["request_id"])
	}
}

// TestRequestLogger_PropagatesClientRequestID verifies a client-supplied
// X-Request-ID is passed through to the request context.
func TestRequestLogger_PropagatesClientRequestID(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := RequestLogger(logger)
	var ctxID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID, _ = r.Context().Value(CtxRequestID).(string)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("X-Request-ID", "client-req-99")
	mw(next).ServeHTTP(httptest.NewRecorder(), req)

	if ctxID != "client-req-99" {
		t.Errorf("CtxRequestID = %q, want client-req-99", ctxID)
	}
}

func TestRequestLogger_LevelFollowsStatus(t *testing.T) {
	cases := []struct {
		status int
		level  string
	}{
		{http.StatusOK, "INFO"},
		{http.StatusNotFound, "WARN"},
		{http.StatusInternalServerError, "ERROR"},
	}
	for _, tc := range cases {
		out := captureRequestLog(t, tc.status)
		if out["level"] != tc.level {
			t.Errorf("status %d: level = %v, want %s", tc.status, out["level"], tc.level)
		}
	}
}

// TestRequestLogger_PreservesFlusher verifies streaming endpoints still get a
// writer that implements http.Flusher behind the request-logging middleware.
func TestRequestLogger_PreservesFlusher(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mw := RequestLogger(logger)

	flushed := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("streaming handler requires http.Flusher behind middleware")
		}
		w.WriteHeader(http.StatusOK)
		fl.Flush()
		flushed = true
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if !flushed {
		t.Fatal("Flush was not forwarded to the underlying writer")
	}
}

// TestRequestLogger_PreservesHijacker verifies Hijacker support is forwarded.
func TestRequestLogger_PreservesHijacker(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	mw := RequestLogger(logger)

	hijacked := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("handler requires http.Hijacker behind middleware")
		}
		hijacked = true
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	mw(next).ServeHTTP(httptest.NewRecorder(), req)
	if !hijacked {
		t.Fatal("Hijacker was not exposed behind middleware")
	}
}

// TestRequestLogger_Implicit200 verifies a handler that writes a body without
// calling WriteHeader is logged as 200 (net/http semantics).
func TestRequestLogger_Implicit200(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	mw := RequestLogger(logger)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	mw(next).ServeHTTP(httptest.NewRecorder(), req)

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON log: %v", err)
	}
	if out["status"] != float64(http.StatusOK) {
		t.Errorf("status = %v, want 200 for implicit WriteHeader", out["status"])
	}
}

package middleware

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// statusRecorder captures the response status code written by the handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

// WriteHeader records the first status code and forwards it. Subsequent
// calls (which net/http tolerates) do not overwrite the recorded status.
func (r *statusRecorder) WriteHeader(code int) {
	if r.status != 0 {
		return
	}
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Write records an implicit 200 when the handler never called WriteHeader.
func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

// Flush forwards SSE flushes. The gateway's streaming endpoints depend on
// the response writer still exposing http.Flusher behind this middleware.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards connection hijacking (used by websocket-style upgrades).
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
}

// Push forwards HTTP/2 server push when supported.
func (r *statusRecorder) Push(target string, opts *http.PushOptions) error {
	if p, ok := r.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// ReadFrom forwards io.ReaderFrom fast paths (e.g. io.Copy to the response).
func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if rf, ok := r.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(struct{ io.Writer }{r.ResponseWriter}, src)
}

// RequestLogger emits one structured log line per HTTP request. It records
// method, path, status, duration, request id (generated when the client did
// not send one), client ip and — when present — the authenticated user and
// API key ids. Request/response bodies are never logged. Log level follows
// the response status: 5xx → error, 4xx → warn, otherwise info.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}

			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}
			// Propagate the same request id downstream (auth, gateway evidence,
			// billing logs) so every layer of a single call shares one id.
			ctx := context.WithValue(r.Context(), CtxRequestID, requestID)
			r = r.WithContext(ctx)

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"request_id", requestID,
				"client_ip", extractIPFromRemoteAddr(r.RemoteAddr),
			}
			if v, _ := r.Context().Value(CtxUserID).(string); v != "" {
				attrs = append(attrs, "user_id", v)
			}
			if v, _ := r.Context().Value(CtxAPIKeyID).(string); v != "" {
				attrs = append(attrs, "api_key_id", v)
			}

			next.ServeHTTP(rec, r)

			attrs = append(attrs, "status", rec.status, "duration_ms", time.Since(start).Milliseconds())
			switch {
			case rec.status >= 500:
				logger.Error("http_request", attrs...)
			case rec.status >= 400:
				logger.Warn("http_request", attrs...)
			default:
				logger.Info("http_request", attrs...)
			}
		})
	}
}

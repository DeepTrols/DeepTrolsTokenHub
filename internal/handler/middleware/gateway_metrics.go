package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/deeptrols/api/internal/pkg/metrics"
)

// GatewayMetrics records the low-cardinality gateway request counters
// (requests_total / success_total / error_total) plus duration for every
// finished request (TH-P05-04). The endpoint label comes from the matched
// route pattern and is clamped to the metrics allowlist, so unmatched or
// parameterized routes can never create high-cardinality series. Counting
// happens AFTER the handler returns and is panic-guarded inside the metrics
// package, so instrumentation can never change a business response.
func GatewayMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			// http.ResponseWriter without an explicit WriteHeader defaults
			// to 200 once written.
			status = http.StatusOK
		}
		metrics.RecordRequest(routeEndpoint(r), status, time.Since(start))
	})
}

// routeEndpoint extracts the matched route pattern (e.g.
// "/v1/chat/completions") for the endpoint label. It runs after the handler,
// so the chi route context is fully resolved.
func routeEndpoint(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return ""
	}
	return rctx.RoutePattern()
}

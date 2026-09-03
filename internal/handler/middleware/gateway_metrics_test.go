package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/deeptrols/api/internal/pkg/metrics"
)

// TestGatewayMetrics_CountsRequestsByOutcome verifies the baseline request
// counters on a real chi router: one success and one server error increment
// exactly the expected low-cardinality series, and the business responses are
// unchanged by the instrumentation.
func TestGatewayMetrics_CountsRequestsByOutcome(t *testing.T) {
	requestsBefore := testutil.ToFloat64(metrics.Default.RequestsTotal.WithLabelValues("chat/completions"))
	successBefore := testutil.ToFloat64(metrics.Default.SuccessTotal.WithLabelValues("chat/completions"))
	modelsRequestsBefore := testutil.ToFloat64(metrics.Default.RequestsTotal.WithLabelValues("models"))
	modelsErrorsBefore := testutil.ToFloat64(metrics.Default.ErrorTotal.WithLabelValues("models", metrics.ReasonServerError))
	otherBefore := testutil.ToFloat64(metrics.Default.RequestsTotal.WithLabelValues("other"))

	r := chi.NewRouter()
	r.Use(GatewayMetrics)
	r.Route("/v1", func(r chi.Router) {
		r.Post("/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		r.Get("/models", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
	})

	// Successful request.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("instrumentation changed the business response: status = %d, want 200", rec.Code)
	}
	// Server error request.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("instrumentation changed the business response: status = %d, want 500", rec.Code)
	}
	// Unmatched route must clamp to the "other" bucket, never leak the path.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/definitely-not-a-route", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("instrumentation changed the business response: status = %d, want 404", rec.Code)
	}

	if got := testutil.ToFloat64(metrics.Default.RequestsTotal.WithLabelValues("chat/completions")) - requestsBefore; got != 1 {
		t.Errorf("requests_total(chat/completions) delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Default.SuccessTotal.WithLabelValues("chat/completions")) - successBefore; got != 1 {
		t.Errorf("success_total(chat/completions) delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Default.RequestsTotal.WithLabelValues("models")) - modelsRequestsBefore; got != 1 {
		t.Errorf("requests_total(models) delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Default.ErrorTotal.WithLabelValues("models", metrics.ReasonServerError)) - modelsErrorsBefore; got != 1 {
		t.Errorf("error_total(models, server_error) delta = %v, want 1", got)
	}
	if got := testutil.ToFloat64(metrics.Default.RequestsTotal.WithLabelValues("other")) - otherBefore; got != 1 {
		t.Errorf("requests_total(other) delta = %v, want 1 (unmatched route clamped)", got)
	}
}

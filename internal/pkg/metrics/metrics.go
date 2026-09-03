// Package metrics implements the P0.5 production observability baseline
// (TH-P05-04): minimal low-cardinality counters for gateway traffic and the
// money path (reserve / settle / release / undercharge / pricing-incomplete /
// provider-before-call blocking).
//
// Label policy (mandated by TH-P05-04):
//
//   - Allowed low-cardinality labels: endpoint, reason_class.
//   - FORBIDDEN anywhere in metric names, labels or values: request_id,
//     user_id, tenant_id, order_no, email, API keys, JWT, prompt text,
//     raw error text, raw URLs, IP addresses.
//
// The setters enforce the policy structurally: every label value passes an
// allowlist sanitizer before it reaches Prometheus, so a future caller cannot
// accidentally leak a high-cardinality or sensitive value.
//
// TH-P05-11 extends the same registry and label policy to the worker lease
// observability baseline (worker_cycles_total / worker_cycle_duration_seconds
// / worker_lease_total): worker names and outcomes are whitelist-clamped by
// SanitizeWorker / SanitizeCycleOutcome / SanitizeLeaseOutcome, and raw lease
// keys or error text never reach any label.
package metrics

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metric names (exported for documentation and tests).
const (
	NameRequestsTotal            = "gateway_requests_total"
	NameSuccessTotal             = "gateway_success_total"
	NameErrorTotal               = "gateway_error_total"
	NameRequestDurationSeconds   = "gateway_request_duration_seconds"
	NameReserveTotal             = "billing_reserve_total"
	NameReserveFailedTotal       = "billing_reserve_failed_total"
	NameSettleTotal              = "billing_settle_total"
	NameSettleFailedTotal        = "billing_settle_failed_total"
	NameReleaseTotal             = "billing_release_total"
	NameReleaseFailedTotal       = "billing_release_failed_total"
	NameUnderchargeFallbackTotal = "billing_undercharge_fallback_total"
	NamePricingIncompleteTotal   = "billing_pricing_incomplete_total"
	NameProviderBlockedTotal     = "gateway_provider_blocked_before_call_total"

	// TH-P05-11: worker lease observability baseline.
	NameWorkerCyclesTotal   = "worker_cycles_total"
	NameWorkerCycleDuration = "worker_cycle_duration_seconds"
	NameWorkerLeaseTotal    = "worker_lease_total"
)

// Allowed reason_class label values (bounded by construction).
const (
	ReasonInsufficientBalance = "insufficient_balance"
	ReasonReserveFailed       = "reserve_failed"
	ReasonPricingIncomplete   = "pricing_incomplete"
	ReasonWalletMissing       = "wallet_missing"
	ReasonTxNotReserved       = "tx_not_reserved"
	ReasonSettleError         = "settle_error"
	ReasonClientError         = "client_error"
	ReasonServerError         = "server_error"
	reasonOther               = "other"
)

// endpointOther is the clamp bucket for any endpoint outside the allowlist.
const endpointOther = "other"

// endpointAliases normalizes short billing-call-site forms onto the canonical
// route-pattern namespace so every series for the same API endpoint shares one
// label value (e.g. settle fallback sites pass "chat"; the route is
// "/v1/chat/completions").
var endpointAliases = map[string]string{
	"chat": "chat/completions",
}

// allowedEndpoints is the complete low-cardinality endpoint label space.
// Anything else (including route patterns with path parameters or arbitrary
// caller strings) is clamped to endpointOther.
var allowedEndpoints = map[string]bool{
	"chat/completions":      true,
	"completions":           true,
	"responses":             true,
	"messages":              true,
	"messages/count_tokens": true,
	"embeddings":            true,
	"images/generations":    true,
	"images/edits":          true,
	"audio/speech":          true,
	"audio/transcriptions":  true,
	"videos/generations":    true,
	"models":                true,
	// Billing call sites use these short forms.
	"chat":  true,
	"video": true,
}

var allowedReasons = map[string]bool{
	ReasonInsufficientBalance: true,
	ReasonReserveFailed:       true,
	ReasonPricingIncomplete:   true,
	ReasonWalletMissing:       true,
	ReasonTxNotReserved:       true,
	ReasonSettleError:         true,
	ReasonClientError:         true,
	ReasonServerError:         true,
}

// TH-P05-11: worker observability label values. The worker whitelist is the
// exact set of leased workers in cmd/worker; any other value (including raw
// lease keys like "worker:lease:reconciler") is clamped to workerOther.
const workerOther = "other"

// Cycle outcome label values (worker_cycles_total).
const (
	WorkerOutcomeSuccess        = "success"
	WorkerOutcomeFailed         = "failed"
	WorkerOutcomePanicRecovered = "panic_recovered"
	WorkerOutcomeSkipped        = "skipped"
)

// Lease outcome label values (worker_lease_total).
const (
	LeaseOutcomeAcquired = "acquired"
	LeaseOutcomeSkipped  = "skipped"
	LeaseOutcomeError    = "error"
)

// AllowedWorkers is the complete low-cardinality worker label space. New
// leased workers must be added here deliberately — a dynamic or unknown name
// is clamped to workerOther, so worker identity can never be smuggled in.
var AllowedWorkers = map[string]bool{
	"health_checker":       true,
	"reconciler":           true,
	"billing_sync":         true,
	"subscription_expirer": true,
	"subscription_renewer": true,
}

var allowedCycleOutcomes = map[string]bool{
	WorkerOutcomeSuccess:        true,
	WorkerOutcomeFailed:         true,
	WorkerOutcomePanicRecovered: true,
	WorkerOutcomeSkipped:        true,
}

var allowedLeaseOutcomes = map[string]bool{
	LeaseOutcomeAcquired: true,
	LeaseOutcomeSkipped:  true,
	LeaseOutcomeError:    true,
}

// SanitizeWorker clamps a worker label value to the allowlist. Raw lease
// keys, hostnames and any other dynamic string become workerOther.
func SanitizeWorker(worker string) string {
	w := strings.TrimSpace(worker)
	if AllowedWorkers[w] {
		return w
	}
	return workerOther
}

// SanitizeCycleOutcome clamps a worker cycle outcome to the allowlist.
func SanitizeCycleOutcome(outcome string) string {
	if allowedCycleOutcomes[outcome] {
		return outcome
	}
	return workerOther
}

// SanitizeLeaseOutcome clamps a lease decision outcome to the allowlist.
func SanitizeLeaseOutcome(outcome string) string {
	if allowedLeaseOutcomes[outcome] {
		return outcome
	}
	return workerOther
}

// SanitizeEndpoint clamps an endpoint label value to the allowlist.
// Route-pattern prefixes ("/v1/") are stripped first; unknown, empty and
// high-cardinality values all become endpointOther.
func SanitizeEndpoint(endpoint string) string {
	e := strings.TrimPrefix(strings.TrimSpace(endpoint), "/v1/")
	if e == "" || strings.ContainsAny(e, "{}*") {
		return endpointOther
	}
	if canonical, ok := endpointAliases[e]; ok {
		return canonical
	}
	if allowedEndpoints[e] {
		return e
	}
	return endpointOther
}

// RequestEndpoint derives the canonical endpoint label from a request path
// (e.g. "/v1/chat/completions" -> "chat/completions"), clamped to the
// allowlist.
func RequestEndpoint(r *http.Request) string {
	if r == nil {
		return endpointOther
	}
	return SanitizeEndpoint(r.URL.Path)
}

// SanitizeReasonClass clamps a reason_class label value to the allowlist.
func SanitizeReasonClass(reason string) string {
	if allowedReasons[reason] {
		return reason
	}
	return reasonOther
}

// StatusClass maps an HTTP status code to a bounded result class.
func StatusClass(status int) string {
	switch {
	case status >= 500:
		return ReasonServerError
	case status >= 400:
		return ReasonClientError
	default:
		return ""
	}
}

// Metrics bundles the P0.5 baseline collectors on a dedicated registry so the
// exposition surface contains only these vetted families (no Go runtime
// collectors, no third-party series).
type Metrics struct {
	Registry *prometheus.Registry

	RequestsTotal          *prometheus.CounterVec   // {endpoint}
	SuccessTotal           *prometheus.CounterVec   // {endpoint}
	ErrorTotal             *prometheus.CounterVec   // {endpoint, reason_class}
	RequestDurationSeconds *prometheus.HistogramVec // {endpoint}

	ReserveTotal       prometheus.Counter
	ReserveFailedTotal *prometheus.CounterVec // {reason_class}
	SettleTotal        prometheus.Counter
	SettleFailedTotal  *prometheus.CounterVec // {reason_class}
	ReleaseTotal       prometheus.Counter
	ReleaseFailedTotal *prometheus.CounterVec // {reason_class}

	UnderchargeFallbackTotal *prometheus.CounterVec // {endpoint}
	PricingIncompleteTotal   *prometheus.CounterVec // {endpoint}
	ProviderBlockedTotal     *prometheus.CounterVec // {endpoint, reason_class}

	// TH-P05-11: worker lease observability baseline.
	WorkerCyclesTotal   *prometheus.CounterVec   // {worker, outcome}
	WorkerCycleDuration *prometheus.HistogramVec // {worker}
	WorkerLeaseTotal    *prometheus.CounterVec   // {worker, outcome}
}

// durationBuckets covers LLM request latencies (sub-second cache hits to
// multi-minute generations) with a bounded bucket count.
var durationBuckets = []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120}

// workerCycleBuckets covers worker cycle durations: sub-second skips and
// fast sync cycles up to long reconciliation runs (bounded bucket count).
var workerCycleBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}

// New builds the metric set on a fresh registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		Registry: reg,
		RequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameRequestsTotal,
			Help: "Gateway requests by endpoint (all outcomes).",
		}, []string{"endpoint"}),
		SuccessTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameSuccessTotal,
			Help: "Gateway requests completed with a 2xx/3xx status.",
		}, []string{"endpoint"}),
		ErrorTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameErrorTotal,
			Help: "Gateway requests completed with a 4xx/5xx status.",
		}, []string{"endpoint", "reason_class"}),
		RequestDurationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    NameRequestDurationSeconds,
			Help:    "Gateway request duration in seconds.",
			Buckets: durationBuckets,
		}, []string{"endpoint"}),
		ReserveTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: NameReserveTotal,
			Help: "Successful wallet reserves (includes idempotent replays; money effect uniqueness is proven by TH-P05-03 W5).",
		}),
		ReserveFailedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameReserveFailedTotal,
			Help: "Failed wallet reserves by bounded reason class.",
		}, []string{"reason_class"}),
		SettleTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: NameSettleTotal,
			Help: "Successful wallet settles at final cost.",
		}),
		SettleFailedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameSettleFailedTotal,
			Help: "Rejected wallet settles by bounded reason class.",
		}, []string{"reason_class"}),
		ReleaseTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: NameReleaseTotal,
			Help: "Successful wallet hold releases (compensation).",
		}),
		ReleaseFailedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameReleaseFailedTotal,
			Help: "Failed wallet hold releases by bounded reason class.",
		}, []string{"reason_class"}),
		UnderchargeFallbackTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameUnderchargeFallbackTotal,
			Help: "Settle fallback events that left evidence of undercharge (TH-P05-02 classes).",
		}, []string{"endpoint"}),
		PricingIncompleteTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NamePricingIncompleteTotal,
			Help: "Requests rejected fail-closed before any reserve because pricing was incomplete.",
		}, []string{"endpoint"}),
		ProviderBlockedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameProviderBlockedTotal,
			Help: "Provider calls prevented by a money-safety gate before any upstream request (surrogate, see docs/OBSERVABILITY_METRICS.md).",
		}, []string{"endpoint", "reason_class"}),
		// TH-P05-11: worker lease observability baseline.
		WorkerCyclesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameWorkerCyclesTotal,
			Help: "Worker cycles by outcome (success / failed / panic_recovered / skipped).",
		}, []string{"worker", "outcome"}),
		WorkerCycleDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    NameWorkerCycleDuration,
			Help:    "Worker cycle duration in seconds (lease attempt included).",
			Buckets: workerCycleBuckets,
		}, []string{"worker"}),
		WorkerLeaseTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: NameWorkerLeaseTotal,
			Help: "Worker lease decisions (acquired / skipped / error). Redis error is fail-closed: the cycle is skipped.",
		}, []string{"worker", "outcome"}),
	}
	reg.MustRegister(
		m.RequestsTotal, m.SuccessTotal, m.ErrorTotal, m.RequestDurationSeconds,
		m.ReserveTotal, m.ReserveFailedTotal,
		m.SettleTotal, m.SettleFailedTotal,
		m.ReleaseTotal, m.ReleaseFailedTotal,
		m.UnderchargeFallbackTotal, m.PricingIncompleteTotal, m.ProviderBlockedTotal,
		m.WorkerCyclesTotal, m.WorkerCycleDuration, m.WorkerLeaseTotal,
	)
	return m
}

// Default is the process-wide instance used by production wiring and tests.
var Default = New()

// Handler exposes /metrics for scraping (Prometheus text format).
func Handler() http.Handler {
	return promhttp.HandlerFor(Default.Registry, promhttp.HandlerOpts{})
}

// safely swallows any instrumentation fault so metrics can never break a
// business request (TH-P05-04 requirement).
func safely(fn func()) {
	defer func() { _ = recover() }()
	fn()
}

// RecordRequest counts one finished gateway request.
func RecordRequest(endpoint string, status int, duration time.Duration) {
	e := SanitizeEndpoint(endpoint)
	safely(func() {
		Default.RequestsTotal.WithLabelValues(e).Inc()
		Default.RequestDurationSeconds.WithLabelValues(e).Observe(duration.Seconds())
		if cls := StatusClass(status); cls != "" {
			Default.ErrorTotal.WithLabelValues(e, cls).Inc()
			return
		}
		Default.SuccessTotal.WithLabelValues(e).Inc()
	})
}

// IncReserve counts one successful reserve.
func IncReserve() { safely(func() { Default.ReserveTotal.Inc() }) }

// IncReserveFailed counts one failed reserve by reason class.
func IncReserveFailed(reasonClass string) {
	safely(func() { Default.ReserveFailedTotal.WithLabelValues(SanitizeReasonClass(reasonClass)).Inc() })
}

// IncSettle counts one successful settle.
func IncSettle() { safely(func() { Default.SettleTotal.Inc() }) }

// IncSettleFailed counts one rejected settle by reason class.
func IncSettleFailed(reasonClass string) {
	safely(func() { Default.SettleFailedTotal.WithLabelValues(SanitizeReasonClass(reasonClass)).Inc() })
}

// IncRelease counts one successful hold release.
func IncRelease() { safely(func() { Default.ReleaseTotal.Inc() }) }

// IncReleaseFailed counts one failed hold release by reason class.
func IncReleaseFailed(reasonClass string) {
	safely(func() { Default.ReleaseFailedTotal.WithLabelValues(SanitizeReasonClass(reasonClass)).Inc() })
}

// IncUnderchargeFallback counts one documented undercharge fallback event.
func IncUnderchargeFallback(endpoint string) {
	safely(func() { Default.UnderchargeFallbackTotal.WithLabelValues(SanitizeEndpoint(endpoint)).Inc() })
}

// IncPricingIncomplete counts one fail-closed pricing-incomplete rejection.
func IncPricingIncomplete(endpoint string) {
	safely(func() { Default.PricingIncompleteTotal.WithLabelValues(SanitizeEndpoint(endpoint)).Inc() })
}

// IncProviderBlocked counts one provider call prevented before any upstream
// request by a money-safety gate.
func IncProviderBlocked(endpoint, reasonClass string) {
	safely(func() {
		Default.ProviderBlockedTotal.WithLabelValues(SanitizeEndpoint(endpoint), SanitizeReasonClass(reasonClass)).Inc()
	})
}

// RecordWorkerCycle counts one finished worker cycle by outcome and observes
// its duration (TH-P05-11). Labels are whitelist-clamped; instrumentation
// faults are swallowed so observability can never break a worker cycle.
func RecordWorkerCycle(worker, outcome string, duration time.Duration) {
	w := SanitizeWorker(worker)
	o := SanitizeCycleOutcome(outcome)
	safely(func() {
		Default.WorkerCyclesTotal.WithLabelValues(w, o).Inc()
		Default.WorkerCycleDuration.WithLabelValues(w).Observe(duration.Seconds())
	})
}

// IncWorkerLease counts one lease decision (acquired / skipped / error) for a
// worker (TH-P05-11).
func IncWorkerLease(worker, outcome string) {
	w := SanitizeWorker(worker)
	o := SanitizeLeaseOutcome(outcome)
	safely(func() {
		Default.WorkerLeaseTotal.WithLabelValues(w, o).Inc()
	})
}

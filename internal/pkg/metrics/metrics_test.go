package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSanitizeEndpoint_AllowedValuesPassThrough pins the bounded endpoint
// label space.
func TestSanitizeEndpoint_AllowedValuesPassThrough(t *testing.T) {
	for _, in := range []string{"chat/completions", "messages", "videos/generations", "video"} {
		if got := SanitizeEndpoint(in); got != in {
			t.Errorf("SanitizeEndpoint(%q) = %q, want %q", in, got, in)
		}
	}
	// Route-pattern prefix is stripped before the allowlist check.
	if got := SanitizeEndpoint("/v1/chat/completions"); got != "chat/completions" {
		t.Errorf("SanitizeEndpoint(/v1/chat/completions) = %q, want chat/completions", got)
	}
	// The short billing-call-site form is aliased onto the canonical route
	// namespace so one endpoint always yields one label value.
	if got := SanitizeEndpoint("chat"); got != "chat/completions" {
		t.Errorf("SanitizeEndpoint(chat) = %q, want chat/completions (alias)", got)
	}
}

// TestSanitizeEndpoint_HighCardinalityClamped proves user-controlled or
// identity-shaped values can never become label values.
func TestSanitizeEndpoint_HighCardinalityClamped(t *testing.T) {
	bad := []string{
		"",
		"550e8400-e29b-41d4-a716-446655440000", // request/user id shape
		"user@example.com",                     // email
		"Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig",   // JWT shape
		"gpt-4o-mini-finetuned-customer-1234567890", // unbounded model-ish string
		"/v1/videos/generations/{id}",               // route pattern with parameter
		"chat/completions?api_key=sk-123",           // raw URL with secret
		"../../../../etc/passwd",                    // traversal noise
	}
	for _, in := range bad {
		if got := SanitizeEndpoint(in); got != endpointOther {
			t.Errorf("SanitizeEndpoint(%q) = %q, want %q", in, got, endpointOther)
		}
	}
}

// TestSanitizeReasonClass pins the bounded reason_class space.
func TestSanitizeReasonClass(t *testing.T) {
	if got := SanitizeReasonClass(ReasonInsufficientBalance); got != ReasonInsufficientBalance {
		t.Errorf("allowed reason mangled: %q", got)
	}
	for _, in := range []string{"", "pq: duplicate key value violates unique constraint", "user_id=123", "timeout after 30s"} {
		if got := SanitizeReasonClass(in); got != reasonOther {
			t.Errorf("SanitizeReasonClass(%q) = %q, want other", in, got)
		}
	}
}

// TestSanitizeWorker pins the bounded worker label space (TH-P05-11): the
// exact leased-worker whitelist passes, everything else — raw lease keys,
// hostnames, dynamic strings — is clamped to "other".
func TestSanitizeWorker(t *testing.T) {
	for _, in := range []string{"health_checker", "reconciler", "billing_sync", "subscription_expirer", "subscription_renewer", "payment_scanner"} {
		if got := SanitizeWorker(in); got != in {
			t.Errorf("SanitizeWorker(%q) = %q, want %q", in, got, in)
		}
	}
	bad := []string{
		"",
		"worker:lease:reconciler", // raw lease key must never become a label
		"host-3.prod.internal",    // hostname
		"pod-7f9c8b6d5-x2k4j",     // pod id
		"user_id=123",
		"reconciler\ninjected_label", // label injection noise
	}
	for _, in := range bad {
		if got := SanitizeWorker(in); got != workerOther {
			t.Errorf("SanitizeWorker(%q) = %q, want %q", in, got, workerOther)
		}
	}
}

// TestSanitizeWorkerOutcomes pins the bounded outcome label spaces.
func TestSanitizeWorkerOutcomes(t *testing.T) {
	for _, in := range []string{WorkerOutcomeSuccess, WorkerOutcomeFailed, WorkerOutcomePanicRecovered, WorkerOutcomeSkipped} {
		if got := SanitizeCycleOutcome(in); got != in {
			t.Errorf("SanitizeCycleOutcome(%q) = %q, want %q", in, got, in)
		}
	}
	for _, in := range []string{LeaseOutcomeAcquired, LeaseOutcomeSkipped, LeaseOutcomeError} {
		if got := SanitizeLeaseOutcome(in); got != in {
			t.Errorf("SanitizeLeaseOutcome(%q) = %q, want %q", in, got, in)
		}
	}
	for _, in := range []string{"", "timeout after 30s", "connection refused", "worker:lease:x"} {
		if got := SanitizeCycleOutcome(in); got != workerOther {
			t.Errorf("SanitizeCycleOutcome(%q) = %q, want %q", in, got, workerOther)
		}
		if got := SanitizeLeaseOutcome(in); got != workerOther {
			t.Errorf("SanitizeLeaseOutcome(%q) = %q, want %q", in, got, workerOther)
		}
	}
}

// TestStatusClass bounds the HTTP result classification.
func TestStatusClass(t *testing.T) {
	cases := map[int]string{200: "", 201: "", 304: "", 400: ReasonClientError, 402: ReasonClientError, 404: ReasonClientError, 500: ReasonServerError, 502: ReasonServerError}
	for status, want := range cases {
		if got := StatusClass(status); got != want {
			t.Errorf("StatusClass(%d) = %q, want %q", status, got, want)
		}
	}
}

// TestGatheredFamilies_NoForbiddenLabels asserts that everything the registry
// can ever expose uses only the allowed label names. This is the structural
// guarantee behind the "no sensitive labels" requirement.
func TestGatheredFamilies_NoForbiddenLabels(t *testing.T) {
	// Exercise every setter so all label combinations materialize.
	RecordRequest("chat/completions", 200, 10*time.Millisecond)
	RecordRequest("evil value with user_id=1", 500, time.Second)
	IncReserve()
	IncReserveFailed("insufficient_balance")
	IncReserveFailed("raw error text that must never leak")
	IncSettle()
	IncSettleFailed(ReasonTxNotReserved)
	IncRelease()
	IncReleaseFailed(ReasonTxNotReserved)
	IncUnderchargeFallback("chat")
	IncPricingIncomplete("chat")
	IncProviderBlocked("chat", ReasonInsufficientBalance)

	// TH-P05-11 worker setters, including hostile inputs that must clamp.
	RecordWorkerCycle("reconciler", WorkerOutcomeSuccess, time.Second)
	RecordWorkerCycle("worker:lease:reconciler", "raw redis error text", time.Millisecond)
	IncWorkerLease("health_checker", LeaseOutcomeAcquired)
	IncWorkerLease("host-1.prod.internal", "error: dial tcp 10.0.0.5:6379: refused")

	// TH-P05-05 alert-support setters, including hostile dependency values
	// that must clamp.
	IncReconciliationCriticalDiff()
	SetDependencyUp(DependencyDatabase, true)
	SetDependencyUp("postgres://user:pass@10.0.0.5:5432/prod", false)

	// TH-P1-CW-01 scan setters, including a hostile channel value.
	AddPaymentOrderScanned("alipay", 2)
	AddPaymentOrderScanEligible("user_id=1", 1)

	// TH-P1-AL-02 create setters, including a hostile outcome value.
	RecordAlipayCreateOrder(AlipayOutcomeSuccess, time.Millisecond)
	RecordAlipayCreateOrder("order_no=DTP1 raw provider text", time.Second)

	allowedLabelNames := map[string]bool{"endpoint": true, "reason_class": true, "worker": true, "outcome": true, "dependency": true, "channel": true}
	allowedLabelValue := func(val string) bool {
		return allowedEndpoints[val] || allowedReasons[val] || AllowedWorkers[val] ||
			allowedCycleOutcomes[val] || allowedLeaseOutcomes[val] || allowedDependencies[val] ||
			allowedPaymentRoutes[val] || allowedAlipayOutcomes[val] ||
			val == endpointOther || val == reasonOther || val == workerOther || val == dependencyOther ||
			val == paymentRouteOther || val == alipayOutcomeOther
	}
	families, err := Default.Registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	if len(families) == 0 {
		t.Fatal("expected metric families, got none")
	}
	for _, fam := range families {
		for _, m := range fam.GetMetric() {
			for _, lp := range m.GetLabel() {
				name := lp.GetName()
				if !allowedLabelNames[name] {
					t.Errorf("metric %s carries forbidden label name %q", fam.GetName(), name)
				}
				val := lp.GetValue()
				if val == "" {
					continue
				}
				if !allowedLabelValue(val) {
					t.Errorf("metric %s carries non-allowlisted label value %q for %s", fam.GetName(), val, name)
				}
			}
		}
	}
}

// TestHandler_Scrapeable verifies the exposition endpoint serves the
// Prometheus text format with the correct Content-Type and contains the
// baseline families (and no sensitive substrings).
func TestHandler_Scrapeable(t *testing.T) {
	IncReserve()
	// CounterVec families only appear after a child exists; materialize one
	// child per worker family (delta-safe for the rest of the suite).
	RecordWorkerCycle("reconciler", WorkerOutcomeSuccess, time.Millisecond)
	IncWorkerLease("reconciler", LeaseOutcomeAcquired)
	// TH-P05-05 families: counter materializes directly; the gauge vec needs
	// one child.
	IncReconciliationCriticalDiff()
	SetDependencyUp(DependencyDatabase, true)
	// TH-P1-04 family: CounterVec needs one child.
	IncPaymentNotifyRouteMismatch("alipay", "epay")
	// TH-P1-AL-01 family: GaugeVec needs one child.
	SetPaymentChannelConfigReady("alipay", true)
	// TH-P1-CW-01 families: CounterVecs need one child each; the hostile
	// channel value must clamp and never reach the exposition.
	AddPaymentOrderScanned("epay", 3)
	AddPaymentOrderScanEligible("order_no=DTP123 hostile", 1)
	// TH-P1-AL-02 families: counter + histogram need one child each.
	RecordAlipayCreateOrder(AlipayOutcomeProviderError, time.Millisecond)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain*", ct)
	}
	body := rec.Body.String()
	for _, name := range []string{
		NameRequestsTotal, NameSuccessTotal, NameErrorTotal,
		NameReserveTotal, NameReserveFailedTotal,
		NameSettleTotal, NameSettleFailedTotal,
		NameReleaseTotal, NameReleaseFailedTotal,
		NameUnderchargeFallbackTotal, NamePricingIncompleteTotal,
		NameProviderBlockedTotal,
		NameWorkerCyclesTotal, NameWorkerCycleDuration, NameWorkerLeaseTotal,
		NameReconciliationCriticalDiffTotal, NameDependencyUp,
		NamePaymentNotifyRouteMismatchTotal,
		NamePaymentChannelConfigReady,
		NamePaymentOrderScanTotal, NamePaymentOrderScanEligibleTotal,
		NameAlipayCreateTotal, NameAlipayCreateDuration,
	} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics body missing family %s", name)
		}
	}
	for _, leak := range []string{"user_id", "request_id", "tenant_id", "order_no", "api_key", "Bearer", "@", "worker:lease:", "postgres://", "10.0.0.5"} {
		if strings.Contains(body, leak) {
			t.Errorf("metrics body contains sensitive substring %q", leak)
		}
	}
}

// TestSanitizePaymentRoute_ClampsToAllowlist verifies the TH-P1-04 label
// sanitizer: only the closed channel set passes; anything else (typos,
// dynamic strings) is clamped to the other bucket.
func TestSanitizePaymentRoute_ClampsToAllowlist(t *testing.T) {
	for _, route := range []string{"epay", "alipay", "wechatpay", " epay "} {
		if got := SanitizePaymentRoute(route); got != strings.TrimSpace(route) {
			t.Errorf("SanitizePaymentRoute(%q) = %q, want %q", route, got, strings.TrimSpace(route))
		}
	}
	for _, route := range []string{"", "bitcoin", "Epay", "DTP20260904"} {
		if got := SanitizePaymentRoute(route); got != "other" {
			t.Errorf("SanitizePaymentRoute(%q) = %q, want other", route, got)
		}
	}
}

// TestSafely_InstrumentationFailureNeverPropagates proves the guard used by
// every setter swallows faults so metrics can never break a business request.
func TestSafely_InstrumentationFailureNeverPropagates(t *testing.T) {
	safely(func() { panic("boom") }) // must not panic the test
}

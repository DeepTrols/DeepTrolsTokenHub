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

	allowedLabelNames := map[string]bool{"endpoint": true, "reason_class": true}
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
				if !allowedEndpoints[val] && !allowedReasons[val] && val != endpointOther && val != reasonOther {
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
	} {
		if !strings.Contains(body, name) {
			t.Errorf("metrics body missing family %s", name)
		}
	}
	for _, leak := range []string{"user_id", "request_id", "tenant_id", "order_no", "api_key", "Bearer", "@"} {
		if strings.Contains(body, leak) {
			t.Errorf("metrics body contains sensitive substring %q", leak)
		}
	}
}

// TestSafely_InstrumentationFailureNeverPropagates proves the guard used by
// every setter swallows faults so metrics can never break a business request.
func TestSafely_InstrumentationFailureNeverPropagates(t *testing.T) {
	safely(func() { panic("boom") }) // must not panic the test
}

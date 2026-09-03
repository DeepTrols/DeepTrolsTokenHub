package gateway

// TH-P05-04 (Basic Gateway Billing Metrics) — integration proofs that real
// gateway requests emit exactly the mandated money-path counters, reusing
// the TH-P05-03 real-Postgres fixture. Counters are read as deltas from the
// process-wide metrics.Default registry (monotonic counters; tests run in
// one process, so before/after deltas isolate each scenario).

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/metrics"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/service/billing"
	gw "github.com/deeptrols/api/internal/service/gateway"
)

// p0504Counter reads a counter (or labeled child) as float for delta asserts.
func p0504Counter(t *testing.T, c prometheus.Collector) float64 {
	t.Helper()
	return testutil.ToFloat64(c)
}

func p0504StandardExecutorFn(f *p0503Fixture) {
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		respBody := validResponseBody()
		usage, _ := usageparser.ParseOpenAIUsage(respBody)
		return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
			UsageSource: usageparser.SourceUpstream}, nil
	}
}

// ---------------------------------------------------------------------------
// AC-01: one successful gateway request emits exactly one reserve and one
// settle event.
// ---------------------------------------------------------------------------

func TestP0504_AC01_SuccessfulRequest_OneReserveOneSettle(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal {
		return hold.Mul(decimal.NewFromInt(2))
	})
	p0504StandardExecutorFn(f)

	reserveBefore := p0504Counter(t, metrics.Default.ReserveTotal)
	settleBefore := p0504Counter(t, metrics.Default.SettleTotal)
	blockedBefore := p0504Counter(t, metrics.Default.ProviderBlockedTotal.WithLabelValues("chat/completions", metrics.ReasonInsufficientBalance))

	req := newNonStreamChatRequest(f.userID, uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if got := p0504Counter(t, metrics.Default.ReserveTotal) - reserveBefore; got != 1 {
		t.Errorf("reserve_total delta = %v, want exactly 1", got)
	}
	if got := p0504Counter(t, metrics.Default.SettleTotal) - settleBefore; got != 1 {
		t.Errorf("settle_total delta = %v, want exactly 1", got)
	}
	if got := p0504Counter(t, metrics.Default.ProviderBlockedTotal.WithLabelValues("chat/completions", metrics.ReasonInsufficientBalance)) - blockedBefore; got != 0 {
		t.Errorf("provider_blocked delta = %v, want 0 on the happy path", got)
	}
	f.assertInvariants(t)
}

// ---------------------------------------------------------------------------
// AC-02: an upstream failure after reserve emits one release event (the
// documented settle-fallback path of TH-P05-02 covers the settle side).
// ---------------------------------------------------------------------------

func TestP0504_AC02_UpstreamFailure_OneReleaseEvent(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal {
		return hold.Mul(decimal.NewFromInt(2))
	})
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		return nil, errors.New("p0504: upstream exploded")
	}

	releaseBefore := p0504Counter(t, metrics.Default.ReleaseTotal)
	reserveBefore := p0504Counter(t, metrics.Default.ReserveTotal)

	req := newNonStreamChatRequest(f.userID, uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body=%s", w.Code, w.Body.String())
	}
	if got := p0504Counter(t, metrics.Default.ReserveTotal) - reserveBefore; got != 1 {
		t.Errorf("reserve_total delta = %v, want 1 (the pre-call hold)", got)
	}
	if got := p0504Counter(t, metrics.Default.ReleaseTotal) - releaseBefore; got != 1 {
		t.Errorf("release_total delta = %v, want exactly 1 (the compensation)", got)
	}
	f.assertInvariants(t)
}

// ---------------------------------------------------------------------------
// AC-03: incomplete pricing emits one pricing_incomplete event and blocks the
// provider call BEFORE any reserve. This variant needs no database: the
// request must die at the fail-closed hold computation.
// ---------------------------------------------------------------------------

func newP0504BrokenPricingApp(t *testing.T) (*app.App, *mockExecutor) {
	t.Helper()
	modelID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive,
				HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID,
				BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o",
				Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return nil, errors.New("p0504: pricing store unavailable")
		},
	}
	executor := &mockExecutor{}
	application := &app.App{
		Config: &config.Config{
			LiteLLM: config.LiteLLMConfig{BaseURL: "http://localhost:9999", MasterKey: "test-master-key"},
		},
		Executor:   executor,
		Router:     gw.NewRouter(modelRepo, channelRepo),
		Pricer:     billing.NewPricer(pricingRepo),
		Charger:    billing.NewCharger(&mockWalletRepo{}),
		Logger:     billing.NewLogger(&mockUsageRepo{}),
		Wallets:    &mockWalletRepo{},
		HttpClient: &http.Client{},
	}
	return application, executor
}

func TestP0504_AC03_PricingIncomplete_NoReserve(t *testing.T) {
	application, executor := newP0504BrokenPricingApp(t)

	pricingBefore := p0504Counter(t, metrics.Default.PricingIncompleteTotal.WithLabelValues("chat/completions"))
	blockedBefore := p0504Counter(t, metrics.Default.ProviderBlockedTotal.WithLabelValues("chat/completions", metrics.ReasonPricingIncomplete))
	reserveBefore := p0504Counter(t, metrics.Default.ReserveTotal)
	reserveFailedBefore := p0504Counter(t, metrics.Default.ReserveFailedTotal.WithLabelValues(metrics.ReasonInsufficientBalance))

	req := newNonStreamChatRequest(uuid.New(), uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 fail-closed; body=%s", w.Code, w.Body.String())
	}
	if got := p0504Counter(t, metrics.Default.PricingIncompleteTotal.WithLabelValues("chat/completions")) - pricingBefore; got != 1 {
		t.Errorf("pricing_incomplete_total delta = %v, want exactly 1", got)
	}
	if got := p0504Counter(t, metrics.Default.ProviderBlockedTotal.WithLabelValues("chat/completions", metrics.ReasonPricingIncomplete)) - blockedBefore; got != 1 {
		t.Errorf("provider_blocked(pricing_incomplete) delta = %v, want exactly 1", got)
	}
	if got := p0504Counter(t, metrics.Default.ReserveTotal) - reserveBefore; got != 0 {
		t.Errorf("reserve_total delta = %v, want 0 (pricing fail-closed precedes reserve)", got)
	}
	if got := p0504Counter(t, metrics.Default.ReserveFailedTotal.WithLabelValues(metrics.ReasonInsufficientBalance)) - reserveFailedBefore; got != 0 {
		t.Errorf("reserve_failed_total delta = %v, want 0", got)
	}
	if executor.executeCalled != 0 {
		t.Errorf("provider called %d times although pricing was incomplete", executor.executeCalled)
	}
}

// ---------------------------------------------------------------------------
// AC-04: an undercharge fallback emits exactly one undercharged event.
// ---------------------------------------------------------------------------

func TestP0504_AC04_Undercharge_OneFallbackEvent(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal { return hold })
	f.executor.executeFn = func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
		respBody := p0503HugeUsageResponseBody()
		usage, _ := usageparser.ParseOpenAIUsage(respBody)
		return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
			UsageSource: usageparser.SourceUpstream}, nil
	}

	fallbackBefore := p0504Counter(t, metrics.Default.UnderchargeFallbackTotal.WithLabelValues("chat/completions"))
	settleFailedBefore := p0504Counter(t, metrics.Default.SettleFailedTotal.WithLabelValues(metrics.ReasonInsufficientBalance))

	req := newNonStreamChatRequest(f.userID, uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (undercharge is evidence, not a client error); body=%s", w.Code, w.Body.String())
	}
	if got := p0504Counter(t, metrics.Default.UnderchargeFallbackTotal.WithLabelValues("chat/completions")) - fallbackBefore; got != 1 {
		t.Errorf("undercharge_fallback_total delta = %v, want exactly 1", got)
	}
	if got := p0504Counter(t, metrics.Default.SettleFailedTotal.WithLabelValues(metrics.ReasonInsufficientBalance)) - settleFailedBefore; got != 1 {
		t.Errorf("settle_failed_total(insufficient_balance) delta = %v, want exactly 1", got)
	}
	f.assertInvariants(t)
}

// ---------------------------------------------------------------------------
// Provider safety surrogate: a zero-balance wallet blocks the provider call
// at the money gate and emits one provider_blocked_before_call event.
// ---------------------------------------------------------------------------

func TestP0504_ProviderBlocked_ZeroBalance_GateCounts(t *testing.T) {
	f := newP0503Fixture(t, func(hold decimal.Decimal) decimal.Decimal { return decimal.Zero })
	p0504StandardExecutorFn(f)

	blockedBefore := p0504Counter(t, metrics.Default.ProviderBlockedTotal.WithLabelValues("chat/completions", metrics.ReasonInsufficientBalance))

	req := newNonStreamChatRequest(f.userID, uuid.New(), validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, f.application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402; body=%s", w.Code, w.Body.String())
	}
	if got := p0504Counter(t, metrics.Default.ProviderBlockedTotal.WithLabelValues("chat/completions", metrics.ReasonInsufficientBalance)) - blockedBefore; got != 1 {
		t.Errorf("provider_blocked(insufficient_balance) delta = %v, want exactly 1", got)
	}
	if f.executor.executeCalled != 0 {
		t.Errorf("provider called although the money gate rejected the request")
	}
}

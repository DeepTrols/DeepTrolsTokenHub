package gateway

// TH-P05-01 — B5 maximum-charge reservation tests.
//
// Unit: max-charge calculator table tests.
// Integration: gateway reserve amount asserted before executor call.
// Regression: pricing-incomplete / wallet-missing remain covered (chat_test.go).
// Failure Injection: missing pricing and malformed max_tokens.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ============================================================================
// Unit: maxHoldOutputTokens table tests (the max-charge output bound)
// ============================================================================

func TestMaxHoldOutputTokens(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]any
		want     int64
		wantMode string
	}{
		{"declared max_tokens", map[string]any{"max_tokens": float64(1000)}, 1000, holdModeDeclaredMax},
		{"max_completion_tokens wins over max_tokens", map[string]any{"max_tokens": float64(500), "max_completion_tokens": float64(2000)}, 2000, holdModeDeclaredMax},
		{"max_completion_tokens malformed falls back to max_tokens", map[string]any{"max_tokens": float64(500), "max_completion_tokens": "oops"}, 500, holdModeDeclaredMax},
		{"absent -> documented fallback cap", map[string]any{}, fallbackOutputTokens, holdModeFallbackCap},
		{"nil value -> fallback", map[string]any{"max_tokens": nil}, fallbackOutputTokens, holdModeFallbackCap},
		{"zero -> fallback", map[string]any{"max_tokens": float64(0)}, fallbackOutputTokens, holdModeFallbackCap},
		{"negative -> fallback", map[string]any{"max_tokens": float64(-5)}, fallbackOutputTokens, holdModeFallbackCap},
		{"non-numeric string -> fallback", map[string]any{"max_tokens": "abc"}, fallbackOutputTokens, holdModeFallbackCap},
		{"numeric string accepted", map[string]any{"max_tokens": "1000"}, 1000, holdModeDeclaredMax},
		{"fractional truncated", map[string]any{"max_tokens": 1000.9}, 1000, holdModeDeclaredMax},
		{"huge declaration capped", map[string]any{"max_tokens": float64(1 << 30)}, maxChargeOutputCap, holdModeDeclaredMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, mode := maxHoldOutputTokens(tt.body)
			if got != tt.want {
				t.Errorf("maxHoldOutputTokens = %d, want %d", got, tt.want)
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
		})
	}
}

// ============================================================================
// Unit: holdUsageFromChatBody keeps the prompt estimate and applies the
// declared output bound.
// ============================================================================

func TestHoldUsageFromChatBody(t *testing.T) {
	msg := "Hello, world! This is a test message with some tokens to count."
	body := map[string]any{
		"model":      "gpt-4o",
		"max_tokens": float64(1000),
		"messages":   []any{map[string]any{"role": "user", "content": msg}},
	}
	hold, mode := holdUsageFromChatBody(body)
	if mode != holdModeDeclaredMax {
		t.Fatalf("mode = %q, want %q", mode, holdModeDeclaredMax)
	}
	wantInput := usageparser.EstimateTextTokens(msg)
	if hold.InputTokens != wantInput {
		t.Errorf("InputTokens = %d, want prompt estimate %d", hold.InputTokens, wantInput)
	}
	if hold.OutputTokens != 1000 {
		t.Errorf("OutputTokens = %d, want declared max 1000", hold.OutputTokens)
	}
	if hold.TotalTokens != hold.InputTokens+hold.OutputTokens {
		t.Errorf("TotalTokens = %d, want %d", hold.TotalTokens, hold.InputTokens+hold.OutputTokens)
	}

	// estimateUsageFromBody must stay untouched for API-key boundary checks.
	base := estimateUsageFromBody(body)
	if base.OutputTokens != estimatedOutputTokens {
		t.Errorf("estimateUsageFromBody.OutputTokens changed: %d, want %d", base.OutputTokens, estimatedOutputTokens)
	}
}

func TestHoldUsageFromChatBody_NoMessages(t *testing.T) {
	hold, mode := holdUsageFromChatBody(map[string]any{"model": "gpt-4o"})
	if mode != holdModeFallbackCap {
		t.Fatalf("mode = %q, want %q", mode, holdModeFallbackCap)
	}
	if hold.OutputTokens != fallbackOutputTokens {
		t.Errorf("OutputTokens = %d, want fallback %d", hold.OutputTokens, fallbackOutputTokens)
	}
	if hold.InputTokens <= 0 {
		t.Errorf("InputTokens = %d, want positive fallback estimate", hold.InputTokens)
	}
}

// ============================================================================
// Unit: reserve amount bucket (low-cardinality observability label)
// ============================================================================

func TestReserveAmountBucket(t *testing.T) {
	tests := []struct {
		amount string
		want   string
	}{
		{"0.0000002", "lt_0.001"},
		{"0.001", "0.001_0.01"},
		{"0.01", "0.01_0.1"},
		{"0.1", "0.1_1"},
		{"1", "gte_1"},
		{"42.5", "gte_1"},
	}
	for _, tt := range tests {
		if got := reserveAmountBucket(decimal.RequireFromString(tt.amount)); got != tt.want {
			t.Errorf("reserveAmountBucket(%s) = %q, want %q", tt.amount, got, tt.want)
		}
	}
}

// ============================================================================
// Shared test environment for max-charge hold integration tests
// ============================================================================

// maxChargeEnv wires the standard mock app; pricing may be swapped per test.
func maxChargeEnv(t *testing.T, pricingRows []domain.ModelPricing, pricingErr error) (*app.App, *mockWalletRepo, *mockExecutor) {
	t.Helper()
	modelID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()

	walletRepo := &mockWalletRepo{}
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			// Reserve must have happened before any upstream call.
			if walletRepo.reserveCalled == 0 {
				t.Error("executor called before wallet reserve")
			}
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-maxcharge", DurationMs: 100,
			}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return pricingRows, pricingErr
		},
	}
	txID := uuid.New()
	walletRepo.findByUserFn = func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
		return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
	}
	walletRepo.reserveFn = func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
		return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
	}
	walletRepo.commitFn = func(ctx context.Context, tID uuid.UUID) error { return nil }

	return newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, &mockUsageRepo{}), walletRepo, executor
}

// expectedMaxChargeHold computes inputPrice*inputTokens/1e6 + outputPrice*outputTokens/1e6
// with the same decimal operations as the pricer.
func expectedMaxChargeHold(inputPrice, outputPrice string, inputTokens, outputTokens int64) decimal.Decimal {
	perM := decimal.NewFromInt(1_000_000)
	return decimal.RequireFromString(inputPrice).Mul(decimal.NewFromInt(inputTokens)).Div(perM).
		Add(decimal.RequireFromString(outputPrice).Mul(decimal.NewFromInt(outputTokens)).Div(perM))
}

// wantHoldWithFloor mirrors the implementation: max-charge formula floored at
// minHoldAmount (AC-03 "never below the previous minimum hold").
func wantHoldWithFloor(inputPrice, outputPrice string, inputTokens, outputTokens int64) decimal.Decimal {
	want := expectedMaxChargeHold(inputPrice, outputPrice, inputTokens, outputTokens)
	if minHold := decimal.RequireFromString(minHoldAmount); want.LessThan(minHold) {
		want = minHold
	}
	return want
}

// realisticPricingEntries prices input/output at 20/80 CNY per 1M tokens so
// the max-charge formula dominates the minimum-hold floor and AC equalities
// are meaningful.
func realisticPricingEntries() []domain.ModelPricing {
	return []domain.ModelPricing{
		{
			ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "input", UnitName: "1M tokens",
			UnitPrice: "20", UpstreamCost: "10", Currency: "CNY", IsActive: true,
		},
		{
			ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "output", UnitName: "1M tokens",
			UnitPrice: "80", UpstreamCost: "40", Currency: "CNY", IsActive: true,
		},
	}
}

func promptEstimateOfValidBody() int64 {
	msg := "Hello, world! This is a test message with some tokens to count."
	return usageparser.EstimateTextTokens(msg)
}

// ============================================================================
// AC-01: max_tokens=1000 + complete pricing -> reserve equals estimated
// input cost + max output cost, before the upstream call.
// ============================================================================

func TestHandleNonStreamingChat_MaxChargeHold_AC01(t *testing.T) {
	application, walletRepo, executor := maxChargeEnv(t, realisticPricingEntries(), nil)
	userID, apiKeyID := uuid.New(), uuid.New()

	body := validRequestBody()
	body["max_tokens"] = float64(1000)

	// Capture log output to verify observability fields (no body text).
	var logBuf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOut)

	req := newNonStreamChatRequest(userID, apiKeyID, body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if executor.executeCalled != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.executeCalled)
	}

	want := expectedMaxChargeHold("20", "80", promptEstimateOfValidBody(), 1000)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("reserve = %s, want input cost + 1000-token output cost = %s", walletRepo.lastReserveAmt, want)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "hold_calc") || !strings.Contains(logs, "mode="+holdModeDeclaredMax) {
		t.Errorf("expected hold_calc observability line with mode=%s, got:\n%s", holdModeDeclaredMax, logs)
	}
	if !strings.Contains(logs, "reserve_bucket=") {
		t.Errorf("expected reserve_bucket label in hold_calc line, got:\n%s", logs)
	}
	if strings.Contains(logs, "test message with some tokens") {
		t.Error("hold observability must not contain request body text")
	}
}

// ============================================================================
// AC-02: missing output pricing -> pricing_incomplete before upstream, no
// reserve transaction.
// ============================================================================

func TestHandleNonStreamingChat_MissingOutputPricing_AC02(t *testing.T) {
	inputOnly := []domain.ModelPricing{{
		ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "input", UnitName: "1M tokens",
		UnitPrice: "0.000015", UpstreamCost: "0.000010", Currency: "CNY", IsActive: true,
	}}
	application, walletRepo, executor := maxChargeEnv(t, inputOnly, nil)
	userID, apiKeyID := uuid.New(), uuid.New()

	body := validRequestBody()
	body["max_tokens"] = float64(1000)

	req := newNonStreamChatRequest(userID, apiKeyID, body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "pricing_incomplete") {
		t.Errorf("body = %s, want pricing_incomplete error", w.Body.String())
	}
	if walletRepo.reserveCalled != 0 {
		t.Errorf("reserve called %d times, want 0 (no reserve transaction)", walletRepo.reserveCalled)
	}
	if executor.executeCalled != 0 {
		t.Errorf("executor called %d times, want 0 (before upstream)", executor.executeCalled)
	}
}

// ============================================================================
// AC-03: max_tokens absent -> documented fallback cap, never below the
// previous minimum hold.
// ============================================================================

func TestHandleNonStreamingChat_AbsentMaxTokens_FallbackCap_AC03(t *testing.T) {
	application, walletRepo, _ := maxChargeEnv(t, realisticPricingEntries(), nil)
	userID, apiKeyID := uuid.New(), uuid.New()

	body := validRequestBody() // no max_tokens

	// Capture the observability line to prove the fallback mode is used.
	var logBuf bytes.Buffer
	prevOut := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOut)

	req := newNonStreamChatRequest(userID, apiKeyID, body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	want := expectedMaxChargeHold("20", "80", promptEstimateOfValidBody(), fallbackOutputTokens)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("reserve = %s, want documented fallback-cap hold %s", walletRepo.lastReserveAmt, want)
	}
	minHold := decimal.RequireFromString(minHoldAmount)
	if walletRepo.lastReserveAmt.LessThan(minHold) {
		t.Errorf("reserve = %s below previous minimum hold %s", walletRepo.lastReserveAmt, minHold)
	}
	if !strings.Contains(logBuf.String(), "mode="+holdModeFallbackCap) {
		t.Errorf("expected hold_calc mode=%s, got:\n%s", holdModeFallbackCap, logBuf.String())
	}
}

// AC-03 floor clause: with (unrealistically) cheap pricing the hold must
// never drop below the previous minimum hold.
func TestHandleNonStreamingChat_CheapPricing_NeverBelowMinimumHold(t *testing.T) {
	application, walletRepo, _ := maxChargeEnv(t, makePricingEntries(), nil)
	userID, apiKeyID := uuid.New(), uuid.New()

	body := validRequestBody()

	req := newNonStreamChatRequest(userID, apiKeyID, body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	minHold := decimal.RequireFromString(minHoldAmount)
	if !walletRepo.lastReserveAmt.Equal(minHold) {
		t.Errorf("reserve = %s, want floor %s when formula is cheaper than the minimum hold", walletRepo.lastReserveAmt, minHold)
	}
}

// ============================================================================
// Failure Injection: malformed max_tokens -> fallback cap hold (no crash,
// no under-reserve to the minimum).
// ============================================================================

func TestHandleNonStreamingChat_MalformedMaxTokens_FallbackCap(t *testing.T) {
	for name, malformed := range map[string]any{
		"non-numeric string": "abc",
		"negative":           float64(-1),
		"null":               nil,
	} {
		t.Run(name, func(t *testing.T) {
			application, walletRepo, _ := maxChargeEnv(t, realisticPricingEntries(), nil)
			userID, apiKeyID := uuid.New(), uuid.New()

			body := validRequestBody()
			body["max_tokens"] = malformed

			req := newNonStreamChatRequest(userID, apiKeyID, body)
			w := httptest.NewRecorder()
			HandleNonStreamingChat(w, req, application, "gpt-4o", body)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
			}
			want := expectedMaxChargeHold("20", "80", promptEstimateOfValidBody(), fallbackOutputTokens)
			if !walletRepo.lastReserveAmt.Equal(want) {
				t.Errorf("reserve = %s, want fallback-cap hold %s", walletRepo.lastReserveAmt, want)
			}
		})
	}
}

// ============================================================================
// Failure Injection: pricer error -> fail-closed, no reserve, no upstream.
// ============================================================================

func TestHandleNonStreamingChat_PricerError_FailsClosed(t *testing.T) {
	application, walletRepo, executor := maxChargeEnv(t, nil, fmt.Errorf("pricing db down"))
	userID, apiKeyID := uuid.New(), uuid.New()

	body := validRequestBody()
	body["max_tokens"] = float64(1000)

	req := newNonStreamChatRequest(userID, apiKeyID, body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code == http.StatusOK {
		t.Fatalf("status = %d; pricer error must fail closed, not serve the request", w.Code)
	}
	if walletRepo.reserveCalled != 0 {
		t.Errorf("reserve called %d times, want 0 on pricer error", walletRepo.reserveCalled)
	}
	if executor.executeCalled != 0 {
		t.Errorf("executor called %d times, want 0 on pricer error", executor.executeCalled)
	}
}

// ============================================================================
// AC-04: streaming and non-streaming use the same hold calculation.
// ============================================================================

func TestHandleStreamingChat_MaxChargeHold_SameAsNonStreaming_AC04(t *testing.T) {
	upstream := claudeStreamUpstream(t) // OpenAI SSE chunks ending with usage
	defer upstream.Close()

	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := claudeStreamApp(t, upstream.URL, walletRepo, usageRepo)
	application.HttpClient = upstream.Client()

	body := map[string]any{
		"model":      "deepseek-chat",
		"stream":     true,
		"max_tokens": float64(1000),
		"messages":   []any{map[string]any{"role": "user", "content": "Hello"}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-max-charge-stream")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleStreamingChat(w, req, application, "deepseek-chat", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Same formula as the non-streaming path: prompt estimate + declared max
	// output at list price, floored at minHoldAmount.
	wantInput := usageparser.EstimateTextTokens("Hello")
	want := wantHoldWithFloor("0.000015", "0.000060", wantInput, 1000)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("stream reserve = %s, want non-streaming-equivalent hold %s", walletRepo.lastReserveAmt, want)
	}

	// Cross-check against the non-streaming path with identical pricing and
	// body: both holds must be byte-for-byte equal.
	applicationNS, walletRepoNS, _ := maxChargeEnv(t, makePricingEntries(), nil)
	bodyNS := map[string]any{
		"model":      "gpt-4o",
		"max_tokens": float64(1000),
		"messages":   []any{map[string]any{"role": "user", "content": "Hello"}},
	}
	reqNS := newNonStreamChatRequest(uuid.New(), uuid.New(), bodyNS)
	wNS := httptest.NewRecorder()
	HandleNonStreamingChat(wNS, reqNS, applicationNS, "gpt-4o", bodyNS)
	if wNS.Code != http.StatusOK {
		t.Fatalf("non-stream status = %d, want %d; body = %s", wNS.Code, http.StatusOK, wNS.Body.String())
	}
	if !walletRepoNS.lastReserveAmt.Equal(walletRepo.lastReserveAmt) {
		t.Errorf("hold mismatch: streaming = %s, non-streaming = %s", walletRepo.lastReserveAmt, walletRepoNS.lastReserveAmt)
	}
}

// ============================================================================
// Chat-shaped relay paths use the same max-charge hold.
// ============================================================================

func TestHandleClaudeMessagesNonStream_MaxChargeHold(t *testing.T) {
	application, walletRepo, _ := maxChargeEnv(t, realisticPricingEntries(), nil)
	userID, apiKeyID := uuid.New(), uuid.New()

	claudeBody := `{"model":"gpt-4o","max_tokens":1000,"messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(claudeBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-claude-max-charge")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleClaudeMessages(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Converted chat body: content "Hello" -> prompt estimate; max_tokens carried.
	want := expectedMaxChargeHold("20", "80", usageparser.EstimateTextTokens("Hello"), 1000)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("claude reserve = %s, want max-charge hold %s", walletRepo.lastReserveAmt, want)
	}
}

func TestHandleResponsesViaChatNonStream_MaxChargeHold(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-r","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"echo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()

	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := responsesChatApp(t, upstream.URL, walletRepo, usageRepo, true, realisticPricingEntries())
	application.HttpClient = upstream.Client()

	body := `{"model":"deepseek-chat","input":"hi","max_output_tokens":1000}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-responses-max-charge")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleResponses(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	// Converted chat body carries max_output_tokens -> max_tokens=1000; the
	// single input word yields a prompt estimate of at least 1 token.
	if walletRepo.reserveCalled == 0 {
		t.Fatal("reserve was not called")
	}
	wantInput := usageparser.EstimateTextTokens("hi")
	if wantInput < 1 {
		wantInput = 1
	}
	want := expectedMaxChargeHold("20", "80", wantInput, 1000)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("responses-via-chat reserve = %s, want max-charge hold %s", walletRepo.lastReserveAmt, want)
	}
}

// Guard: the JSON round-trip used by relay bodies must keep numeric
// max_tokens decodable by the calculator.
func TestMaxHoldOutputTokens_JSONRoundTrip(t *testing.T) {
	var body map[string]any
	if err := json.Unmarshal([]byte(`{"max_tokens":1000}`), &body); err != nil {
		t.Fatal(err)
	}
	got, mode := maxHoldOutputTokens(body)
	if got != 1000 || mode != holdModeDeclaredMax {
		t.Errorf("round-trip max_tokens = (%d, %s), want (1000, %s)", got, mode, holdModeDeclaredMax)
	}
}

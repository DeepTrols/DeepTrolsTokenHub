package gateway

// TH-P05-12 — B5 non-chat / multimodal coverage closure tests.
//
// Stage 1 (RED): these tests assert the SAFE behavior (maximum-charge or
// fail-closed reserves) against the old silent-minHold code paths and must
// FAIL before the implementation.
//
// Coverage: C-1 (/v1/responses + /v1/completions empty estimates), C-2
// (silent minHoldAmount fallback on pricer error in the forwarded pipelines),
// C-3 (chat multimodal content parts not counted in the input estimate).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// tokenPricingPerMillion prices input/output per 1M tokens (20/80 CNY) so
// max-charge equalities are meaningful (formula dominates the floor).
func tokenPricingPerMillion() []domain.ModelPricing {
	return []domain.ModelPricing{
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "input", UnitName: "1M tokens", UnitPrice: "20", UpstreamCost: "10", Currency: "CNY", IsActive: true},
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "output", UnitName: "1M tokens", UnitPrice: "80", UpstreamCost: "40", Currency: "CNY", IsActive: true},
	}
}

// newEndpointEnvPricing is newEndpointEnv with injectable pricing rows/error.
func newEndpointEnvPricing(executor gw.Executor, rows []domain.ModelPricing, pricingErr error) (*app.App, *mockWalletRepo, *mockUsageRepo) {
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Name: "ch1", Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 100, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "test", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return rows, pricingErr
		},
	}
	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		releaseFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}
	return newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo), walletRepo, usageRepo
}

// okExecutor returns a successful empty-usage response for JSON endpoints.
// reserveCount must report the live wallet's reserve counter so the AC-07
// guard fires against the real mock used by the app under test.
func okExecutor(reserveCount func() int) *mockExecutor {
	return &mockExecutor{
		executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
			// AC-07/AC-09: the provider must never be called before reserve.
			if reserveCount != nil && reserveCount() == 0 {
				panic("provider called before wallet reserve")
			}
			return &gw.ExecuteResponse{
				StatusCode:  http.StatusOK,
				Body:        map[string]any{"ok": true},
				Usage:       &usageparser.NormalizedUsage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
				UsageSource: usageparser.SourceUpstream,
				DurationMs:  5,
			}, nil
		},
	}
}

// ============================================================================
// C-1: /v1/completions must reserve prompt cost + declared max output, not a
// silent minimum hold (AC-03).
// ============================================================================

func TestHandleCompletions_MaxChargeHold_NotSilentMinHold(t *testing.T) {
	var walletRepo *mockWalletRepo
	executor := okExecutor(func() int {
		if walletRepo == nil {
			return 0
		}
		return walletRepo.reserveCalled
	})
	application, walletRepo, _ := newEndpointEnvPricing(executor, tokenPricingPerMillion(), nil)

	prompt := "Write a haiku about the sea"
	body := map[string]any{
		"model":      "gpt-3.5-turbo-instruct",
		"prompt":     prompt,
		"max_tokens": float64(1000),
	}
	req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/completions", body)
	w := httptest.NewRecorder()
	HandleCompletions(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	want := expectedMaxChargeHold("20", "80", usageparser.EstimateTextTokens(prompt), 1000)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("completions reserve = %s, want max-charge hold %s (prompt cost + declared max output), not a silent minimum hold",
			walletRepo.lastReserveAmt, want)
	}
	if walletRepo.lastReserveAmt.Equal(decimal.RequireFromString(minHoldAmount)) {
		t.Error("completions reserve equals silent minHoldAmount — C-1 hole still present")
	}
}

// ============================================================================
// C-1: /v1/responses polymorphic input must be priced (AC-02).
// ============================================================================

func TestHandleResponses_StringInput_MaxChargeHold(t *testing.T) {
	application, walletRepo, _ := newEndpointEnvPricing(okExecutor(nil), tokenPricingPerMillion(), nil)

	body := map[string]any{
		"model":             "gpt-4o",
		"input":             "hello world from responses",
		"max_output_tokens": float64(1000),
	}
	req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/responses", body)
	w := httptest.NewRecorder()
	HandleResponses(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	want := expectedMaxChargeHold("20", "80", usageparser.EstimateTextTokens("hello world from responses"), 1000)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("responses reserve = %s, want max-charge hold %s from polymorphic input estimate",
			walletRepo.lastReserveAmt, want)
	}
}

func TestHandleResponses_ItemListInput_CountsTextParts(t *testing.T) {
	application, walletRepo, _ := newEndpointEnvPricing(okExecutor(nil), tokenPricingPerMillion(), nil)

	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{
				"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_text", "text": "count these tokens"}},
			},
		},
		"max_output_tokens": float64(500),
	}
	req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/responses", body)
	w := httptest.NewRecorder()
	HandleResponses(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	want := expectedMaxChargeHold("20", "80", usageparser.EstimateTextTokens("count these tokens"), 500)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("responses reserve = %s, want %s (input_text parts must count)", walletRepo.lastReserveAmt, want)
	}
}

// ============================================================================
// C-2: pricer error / missing dimension must fail closed on EVERY forwarded
// pipeline — never silently reserve minHoldAmount.
// ============================================================================

func TestHandleEmbeddings_PricerError_FailsClosed(t *testing.T) {
	for name, tc := range map[string]struct {
		rows []domain.ModelPricing
		err  error
	}{
		"pricer error":    {nil, context.DeadlineExceeded},
		"missing pricing": {nil, nil},
	} {
		t.Run(name, func(t *testing.T) {
			executor := &mockExecutor{}
			application, walletRepo, _ := newEndpointEnvPricing(executor, tc.rows, tc.err)

			req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/embeddings",
				map[string]any{"model": "text-embedding-3-small", "input": "price me"})
			w := httptest.NewRecorder()
			HandleEmbeddings(application).ServeHTTP(w, req)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422 fail-closed; body = %s", w.Code, w.Body.String())
			}
			if walletRepo.reserveCalled != 0 {
				t.Errorf("reserve called %d times, want 0", walletRepo.reserveCalled)
			}
			if executor.executeEndpointCalled != 0 {
				t.Errorf("provider called %d times, want 0", executor.executeEndpointCalled)
			}
		})
	}
}

func TestHandleAudioSpeech_PricerError_FailsClosed(t *testing.T) {
	executor := &mockExecutor{}
	application, walletRepo, _ := newEndpointEnvPricing(executor, nil, context.DeadlineExceeded)

	req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/audio/speech",
		map[string]any{"model": "tts-1", "input": "say hi"})
	w := httptest.NewRecorder()
	HandleAudioSpeech(application).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 fail-closed; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeRawCalled != 0 {
		t.Errorf("reserve=%d raw-calls=%d, want 0/0 on pricer error", walletRepo.reserveCalled, executor.executeRawCalled)
	}
}

func TestHandleAudioTranscriptions_MissingAudioPricing_FailsClosed(t *testing.T) {
	executor := &mockExecutor{}
	// Pricing rows without the "audio" dimension -> MissingPricing.
	application, walletRepo, _ := newEndpointEnvPricing(executor, tokenPricingPerMillion(), nil)

	req := newMultipartEndpointRequest(uuid.New(), uuid.New(), "/v1/audio/transcriptions",
		map[string]string{"model": "whisper-1"},
		map[string]struct {
			name, contentType string
			content           []byte
		}{
			"file": {name: "a.mp3", contentType: "audio/mpeg", content: make([]byte, 32000)},
		})
	w := httptest.NewRecorder()
	HandleAudioTranscriptions(application).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 fail-closed; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeMultipartCalled != 0 {
		t.Errorf("reserve=%d multipart-calls=%d, want 0/0", walletRepo.reserveCalled, executor.executeMultipartCalled)
	}
}

func TestHandleVideoGenerations_PricerError_FailsClosed(t *testing.T) {
	executor := &mockExecutor{}
	application, walletRepo, _ := newEndpointEnvPricing(executor, nil, context.DeadlineExceeded)

	req := newVideoRequest(uuid.New(), uuid.New(), map[string]any{
		"model": "doubao-seedance", "prompt": "a cat", "n": float64(1),
	})
	w := httptest.NewRecorder()
	HandleVideoGenerations(application).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 fail-closed; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeEndpointCalled != 0 {
		t.Errorf("reserve=%d provider-calls=%d, want 0/0 on pricer error", walletRepo.reserveCalled, executor.executeEndpointCalled)
	}
}

func TestHandleCompletions_PricerError_FailsClosed(t *testing.T) {
	executor := &mockExecutor{}
	application, walletRepo, _ := newEndpointEnvPricing(executor, nil, context.DeadlineExceeded)

	req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/completions",
		map[string]any{"model": "gpt-3.5-turbo-instruct", "prompt": "hi", "max_tokens": float64(10)})
	w := httptest.NewRecorder()
	HandleCompletions(application).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 fail-closed; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeEndpointCalled != 0 {
		t.Errorf("reserve=%d provider-calls=%d, want 0/0", walletRepo.reserveCalled, executor.executeEndpointCalled)
	}
}

// ============================================================================
// C-3: chat multimodal content parts must count into the input estimate
// (AC-06); unpriceable parts (file refs / unknown) fail closed.
// ============================================================================

func TestHandleChatCompletions_MultimodalParts_CountedInHold(t *testing.T) {
	application, walletRepo, executor := maxChargeEnv(t, realisticPricingEntries(), nil)

	text := "describe this image"
	body := map[string]any{
		"model":      "gpt-4o",
		"max_tokens": float64(1000),
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": text},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://img.example/x.png"}},
			},
		}},
	}
	req := newNonStreamChatRequest(uuid.New(), uuid.New(), body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if executor.executeCalled != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.executeCalled)
	}
	// Text tokens + the safe image allowance (3072) must both be priced.
	want := expectedMaxChargeHold("20", "80", usageparser.EstimateTextTokens(text)+3072, 1000)
	if !walletRepo.lastReserveAmt.Equal(want) {
		t.Errorf("multimodal reserve = %s, want %s (text + image allowance)", walletRepo.lastReserveAmt, want)
	}
}

func TestHandleChatCompletions_FileContentPart_FailsClosed(t *testing.T) {
	application, walletRepo, executor := maxChargeEnv(t, realisticPricingEntries(), nil)

	body := map[string]any{
		"model":      "gpt-4o",
		"max_tokens": float64(1000),
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": "summarize"},
				map[string]any{"type": "input_file", "file_url": map[string]any{"url": "https://files.example/big.pdf"}},
			},
		}},
	}
	req := newNonStreamChatRequest(uuid.New(), uuid.New(), body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 fail-closed for unbounded file content; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeCalled != 0 {
		t.Errorf("reserve=%d provider-calls=%d, want 0/0", walletRepo.reserveCalled, executor.executeCalled)
	}
}

// ============================================================================
// Estimator unit tests: real pricing units and worst-case bases (AC-04, AC-08).
// ============================================================================

func TestEstimateSTTUsage_WorstCaseDuration(t *testing.T) {
	// A 750 KB upload at the worst plausible codec rate (~6 kbps opus =
	// 750 B/s) is up to 1000 seconds — that maximum, not the 128 kbps
	// assumption, must be the billing basis.
	nu := estimateSTTUsage(map[string]any{"_file_size": int64(750_000)})
	if nu.AudioSeconds != 1000 {
		t.Errorf("AudioSeconds = %d, want worst-case 1000", nu.AudioSeconds)
	}
	// Sub-floor sizes bill one second minimum (never zero, never silent).
	if got := estimateSTTUsage(map[string]any{"_file_size": int64(10)}).AudioSeconds; got != 1 {
		t.Errorf("AudioSeconds = %d, want floor 1", got)
	}
}

func TestEstimateEmbeddingsUsage_TokenArrays(t *testing.T) {
	// Token-id array inputs (OpenAI embeddings accept [[1,2,3]]) must count
	// one token per id instead of silently dropping to the minimum.
	nu := estimateEmbeddingsUsage(map[string]any{"input": []any{
		"some text",
		[]any{float64(1), float64(2), float64(3)},
	}})
	want := usageparser.EstimateTextTokens("some text") + 3
	if nu.InputTokens != want {
		t.Errorf("InputTokens = %d, want %d (text + token-array ids)", nu.InputTokens, want)
	}
}

func TestEstimateImagesEditsUsage_CountsRequestedEdits(t *testing.T) {
	nu := estimateImagesEditsUsage(map[string]any{"n": "3"})
	if nu.ImageCount != 3 {
		t.Errorf("ImageCount = %d, want 3", nu.ImageCount)
	}
	nu = estimateImagesEditsUsage(map[string]any{})
	if nu.ImageCount != 1 {
		t.Errorf("ImageCount default = %d, want 1", nu.ImageCount)
	}
}

// ============================================================================
// AC-09 malformed-request cases: invalid input is rejected before any
// reserve or provider call — never billed from a broken estimate.
// ============================================================================

func TestHandleCompletions_MissingPrompt_Rejected(t *testing.T) {
	executor := &mockExecutor{}
	application, walletRepo, _ := newEndpointEnvPricing(executor, tokenPricingPerMillion(), nil)

	req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/completions",
		map[string]any{"model": "gpt-3.5-turbo-instruct", "max_tokens": float64(10)})
	w := httptest.NewRecorder()
	HandleCompletions(application).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for missing prompt; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeEndpointCalled != 0 {
		t.Errorf("reserve=%d provider-calls=%d, want 0/0", walletRepo.reserveCalled, executor.executeEndpointCalled)
	}
}

func TestHandleResponses_UnpriceableInputItem_FailsClosed(t *testing.T) {
	executor := &mockExecutor{}
	application, walletRepo, _ := newEndpointEnvPricing(executor, tokenPricingPerMillion(), nil)

	body := map[string]any{
		"model": "gpt-4o",
		"input": []any{
			map[string]any{
				"type": "message", "role": "user",
				"content": []any{map[string]any{"type": "input_file", "file_url": map[string]any{"url": "https://files.example/doc.pdf"}}},
			},
		},
		"max_output_tokens": float64(100),
	}
	req := newEndpointRequest(uuid.New(), uuid.New(), "/v1/responses", body)
	w := httptest.NewRecorder()
	HandleResponses(application).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 fail-closed for unbounded file input; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeEndpointCalled != 0 {
		t.Errorf("reserve=%d provider-calls=%d, want 0/0", walletRepo.reserveCalled, executor.executeEndpointCalled)
	}
}

func TestHandleImagesEdits_InvalidCount_Rejected(t *testing.T) {
	executor := &mockExecutor{}
	application, walletRepo, _ := newEndpointEnvPricing(executor, nil, nil)

	req := newMultipartEndpointRequest(uuid.New(), uuid.New(), "/v1/images/edits",
		map[string]string{"model": "gpt-image-1", "n": "25"},
		map[string]struct {
			name, contentType string
			content           []byte
		}{
			"image": {name: "i.png", contentType: "image/png", content: []byte("fake")},
		})
	w := httptest.NewRecorder()
	HandleImagesEdits(application).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for n=25 (range 1..10); body = %s", w.Code, w.Body.String())
	}
	if walletRepo.reserveCalled != 0 || executor.executeMultipartCalled != 0 {
		t.Errorf("reserve=%d multipart-calls=%d, want 0/0", walletRepo.reserveCalled, executor.executeMultipartCalled)
	}
}

func TestEstimateUsageFromBody_MultimodalParts(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "plain string"},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "part text"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "u"}},
				map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "x"}},
			}},
		},
	}
	nu := estimateUsageFromBody(body)
	want := usageparser.EstimateTextTokens("plain string") +
		usageparser.EstimateTextTokens("part text") + 3072 + 4096
	if nu.InputTokens != want {
		t.Errorf("InputTokens = %d, want %d (string + text part + image allowance + audio allowance)", nu.InputTokens, want)
	}
}

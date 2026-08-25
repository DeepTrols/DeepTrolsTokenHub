package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// makeEndpointPricingEntries returns pricing for every dimension the forwarded
// endpoints can bill: input/output tokens, image count, audio seconds and TTS
// characters.
func makeEndpointPricingEntries() []domain.ModelPricing {
	return []domain.ModelPricing{
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "input", UnitName: "token", UnitPrice: "0.000015", UpstreamCost: "0.000010", Currency: "CNY", IsActive: true},
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "output", UnitName: "token", UnitPrice: "0.000060", UpstreamCost: "0.000040", Currency: "CNY", IsActive: true},
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "image", UnitName: "image", UnitPrice: "0.050000", UpstreamCost: "0.040000", Currency: "CNY", IsActive: true},
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "audio", UnitName: "second", UnitPrice: "0.001000", UpstreamCost: "0.000800", Currency: "CNY", IsActive: true},
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "tts", UnitName: "character", UnitPrice: "0.000010", UpstreamCost: "0.000008", Currency: "CNY", IsActive: true},
		{ID: uuid.New(), ModelID: uuid.Nil, PricingDimension: "video", UnitName: "video", UnitPrice: "0.080000", UpstreamCost: "0.060000", Currency: "CNY", IsActive: true},
	}
}

// newEndpointEnv builds the standard mock app with endpoint-capable pricing.
func newEndpointEnv(executor gw.Executor) (*app.App, *mockWalletRepo, *mockUsageRepo) {
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
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makeEndpointPricingEntries(), nil
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

// newEndpointRequest builds a POST request with auth context and JSON body.
func newEndpointRequest(userID, apiKeyID uuid.UUID, path string, body map[string]any) *http.Request {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-ep-"+uuid.New().String())
	return setAuthContext(req, userID, apiKeyID)
}

// newMultipartEndpointRequest builds a POST request with auth context and a
// multipart body containing the given fields and files.
func newMultipartEndpointRequest(userID, apiKeyID uuid.UUID, path string, fields map[string]string, files map[string]struct {
	name, contentType string
	content           []byte
}) *http.Request {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	for name, f := range files {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, name, f.name))
		h.Set("Content-Type", f.contentType)
		part, _ := w.CreatePart(h)
		_, _ = part.Write(f.content)
	}
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Request-ID", "test-mp-"+uuid.New().String())
	return setAuthContext(req, userID, apiKeyID)
}

func TestHandleAudioTranscriptions_Success(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeMultipartFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{
				StatusCode:  http.StatusOK,
				Body:        map[string]any{"text": "你好"},
				Usage:       &usageparser.NormalizedUsage{},
				UsageSource: usageparser.SourceEstimated,
				DurationMs:  30,
			}, nil
		},
	}
	application, walletRepo, usageRepo := newEndpointEnv(executor)

	req := newMultipartEndpointRequest(userID, apiKeyID, "/v1/audio/transcriptions",
		map[string]string{"model": "whisper-1", "language": "zh"},
		map[string]struct {
			name, contentType string
			content           []byte
		}{
			"file": {name: "demo.mp3", contentType: "audio/mpeg", content: []byte("fake-audio-bytes")},
		},
	)
	w := httptest.NewRecorder()
	HandleAudioTranscriptions(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if executor.lastEndpoint != "audio/transcriptions" {
		t.Errorf("lastEndpoint = %q, want audio/transcriptions", executor.lastEndpoint)
	}
	if got := executor.lastMultipartFiles["file"].Content; string(got) != "fake-audio-bytes" {
		t.Errorf("file content = %q, want fake-audio-bytes", string(got))
	}
	if walletRepo.settleCalled != 1 {
		t.Errorf("settleCalled = %d, want 1", walletRepo.settleCalled)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.RequestType != "audio" {
		t.Errorf("RequestType = %q, want audio", log.RequestType)
	}
	if len(usageRepo.lastChargeLines) == 0 || usageRepo.lastChargeLines[0].Dimension != "audio" {
		t.Errorf("charge lines = %+v, want first dimension audio", usageRepo.lastChargeLines)
	}
	if log.UsageNormalized["audio_seconds"] != float64(1) {
		t.Errorf("UsageNormalized = %+v, want audio_seconds 1", log.UsageNormalized)
	}
}

func TestHandleAudioTranscriptions_PlainTextRelay(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeMultipartFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{
				StatusCode:  http.StatusOK,
				Body:        map[string]any{"text": "transcribed"},
				Usage:       &usageparser.NormalizedUsage{},
				UsageSource: usageparser.SourceEstimated,
				DurationMs:  10,
			}, nil
		},
	}
	application, _, _ := newEndpointEnv(executor)

	req := newMultipartEndpointRequest(userID, apiKeyID, "/v1/audio/transcriptions",
		map[string]string{"model": "whisper-1"},
		map[string]struct {
			name, contentType string
			content           []byte
		}{
			"file": {name: "demo.mp3", contentType: "audio/mpeg", content: []byte("fake-audio-bytes")},
		},
	)
	w := httptest.NewRecorder()
	HandleAudioTranscriptions(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	if w.Body.String() != "transcribed" {
		t.Errorf("body = %q, want transcribed", w.Body.String())
	}
}

func TestHandleImagesEdits_Success(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeMultipartFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{
				StatusCode:  http.StatusOK,
				Body:        map[string]any{"created": int64(1), "data": []any{map[string]any{"url": "https://img/x"}}},
				Usage:       &usageparser.NormalizedUsage{},
				UsageSource: usageparser.SourceEstimated,
				DurationMs:  40,
			}, nil
		},
	}
	application, walletRepo, usageRepo := newEndpointEnv(executor)

	req := newMultipartEndpointRequest(userID, apiKeyID, "/v1/images/edits",
		map[string]string{"model": "dall-e-2", "prompt": "add a hat"},
		map[string]struct {
			name, contentType string
			content           []byte
		}{
			"image": {name: "pic.png", contentType: "image/png", content: []byte("png-bytes")},
		},
	)
	w := httptest.NewRecorder()
	HandleImagesEdits(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if executor.lastEndpoint != "images/edits" {
		t.Errorf("lastEndpoint = %q, want images/edits", executor.lastEndpoint)
	}
	if walletRepo.settleCalled != 1 {
		t.Errorf("settleCalled = %d, want 1", walletRepo.settleCalled)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.RequestType != "images" {
		t.Errorf("RequestType = %q, want images", log.RequestType)
	}
	if len(usageRepo.lastChargeLines) == 0 || usageRepo.lastChargeLines[0].Dimension != "image" {
		t.Errorf("charge lines = %+v, want first dimension image", usageRepo.lastChargeLines)
	}
}

func TestHandleAudioTranscriptions_MissingModelOrFile(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{}
	application, _, _ := newEndpointEnv(executor)

	t.Run("missing model", func(t *testing.T) {
		req := newMultipartEndpointRequest(userID, apiKeyID, "/v1/audio/transcriptions",
			map[string]string{},
			map[string]struct {
				name, contentType string
				content           []byte
			}{
				"file": {name: "demo.mp3", contentType: "audio/mpeg", content: []byte("fake-audio-bytes")},
			},
		)
		w := httptest.NewRecorder()
		HandleAudioTranscriptions(application).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing file", func(t *testing.T) {
		req := newMultipartEndpointRequest(userID, apiKeyID, "/v1/audio/transcriptions",
			map[string]string{"model": "whisper-1"},
			nil,
		)
		w := httptest.NewRecorder()
		HandleAudioTranscriptions(application).ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleEmbeddings_Success_UpstreamUsage(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := map[string]any{
				"object": "list",
				"data":   []any{map[string]any{"embedding": []any{float64(0.1), float64(0.2)}}},
				"usage":  map[string]any{"prompt_tokens": float64(10), "total_tokens": float64(10)},
			}
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage, UsageSource: usageparser.SourceUpstream, ProviderReqID: "emb-test-1", DurationMs: 9}, nil
		},
	}
	application, walletRepo, usageRepo := newEndpointEnv(executor)

	req := newEndpointRequest(userID, apiKeyID, "/v1/embeddings", map[string]any{
		"model": "text-embedding-3-small",
		"input": "hello world",
	})
	w := httptest.NewRecorder()
	HandleEmbeddings(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if executor.lastEndpoint != "embeddings" {
		t.Errorf("lastEndpoint = %q, want embeddings", executor.lastEndpoint)
	}
	if walletRepo.settleCalled != 1 {
		t.Errorf("settleCalled = %d, want 1", walletRepo.settleCalled)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.RequestType != "embeddings" {
		t.Errorf("RequestType = %q, want embeddings", log.RequestType)
	}
	if log.UsageSource != domain.UsageSourceUpstream {
		t.Errorf("UsageSource = %s, want upstream", log.UsageSource)
	}
	if log.Status != domain.UsageLogStatusCompleted {
		t.Errorf("Status = %s, want completed", log.Status)
	}
	if len(usageRepo.lastChargeLines) == 0 || usageRepo.lastChargeLines[0].Dimension != "input" {
		t.Errorf("charge lines = %+v, want first dimension input", usageRepo.lastChargeLines)
	}
}

func TestHandleEmbeddings_NoUsage_Estimated(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: map[string]any{"object": "list", "data": []any{}}, Usage: &usageparser.NormalizedUsage{}, UsageSource: usageparser.SourceEstimated, DurationMs: 5}, nil
		},
	}
	application, _, usageRepo := newEndpointEnv(executor)

	req := newEndpointRequest(userID, apiKeyID, "/v1/embeddings", map[string]any{
		"model": "text-embedding-3-small",
		"input": "some text without upstream usage",
	})
	w := httptest.NewRecorder()
	HandleEmbeddings(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	log := waitForUsageLog(t, usageRepo)
	if log.UsageSource != domain.UsageSourceEstimated {
		t.Errorf("UsageSource = %s, want estimated", log.UsageSource)
	}
	if log.Status != domain.UsageLogStatusCompleted {
		t.Errorf("Status = %s, want completed", log.Status)
	}
}

func TestHandleImagesGenerations_Success_ImageCount(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: map[string]any{"created": int64(1), "data": []any{map[string]any{"url": "https://img/x"}}}, Usage: &usageparser.NormalizedUsage{}, UsageSource: usageparser.SourceEstimated, DurationMs: 30}, nil
		},
	}
	application, walletRepo, usageRepo := newEndpointEnv(executor)

	req := newEndpointRequest(userID, apiKeyID, "/v1/images/generations", map[string]any{
		"model": "dall-e-3",
		"n":     float64(2),
	})
	w := httptest.NewRecorder()
	HandleImagesGenerations(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.settleCalled != 1 {
		t.Errorf("settleCalled = %d, want 1", walletRepo.settleCalled)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.RequestType != "images" {
		t.Errorf("RequestType = %q, want images", log.RequestType)
	}
	if len(usageRepo.lastChargeLines) == 0 || usageRepo.lastChargeLines[0].Dimension != "image" {
		t.Errorf("charge lines = %+v, want first dimension image", usageRepo.lastChargeLines)
	}
	if log.UsageNormalized["image_count"] != float64(2) {
		t.Errorf("UsageNormalized = %+v, want image_count 2", log.UsageNormalized)
	}
}

func TestHandleAudioSpeech_Success_TTSCharacters(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeRawFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.RawResponse, error) {
			return &gw.RawResponse{StatusCode: http.StatusOK, ContentType: "audio/mpeg", Body: []byte("audio-binary-data"), ProviderReqID: "tts-test-1", DurationMs: 20}, nil
		},
	}
	application, walletRepo, usageRepo := newEndpointEnv(executor)

	req := newEndpointRequest(userID, apiKeyID, "/v1/audio/speech", map[string]any{
		"model": "tts-1",
		"input": "hello world, this is a speech test",
	})
	w := httptest.NewRecorder()
	HandleAudioSpeech(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "audio-binary-data" {
		t.Errorf("relayed body = %q, want raw audio bytes", got)
	}
	if ct := w.Header().Get("Content-Type"); ct != "audio/mpeg" {
		t.Errorf("Content-Type = %q, want audio/mpeg", ct)
	}
	if walletRepo.settleCalled != 1 {
		t.Errorf("settleCalled = %d, want 1", walletRepo.settleCalled)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.RequestType != "audio" {
		t.Errorf("RequestType = %q, want audio", log.RequestType)
	}
	if len(usageRepo.lastChargeLines) == 0 || usageRepo.lastChargeLines[0].Dimension != "tts" {
		t.Errorf("charge lines = %+v, want first dimension tts", usageRepo.lastChargeLines)
	}
	chars, _ := log.UsageNormalized["tts_characters"].(float64)
	if chars <= 0 {
		t.Errorf("UsageNormalized = %+v, want tts_characters > 0", log.UsageNormalized)
	}
}

func TestHandleForwardedEndpoint_UpstreamHTTPError_LogsFailed(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		body    map[string]any
		handler func(*app.App) http.HandlerFunc
		wantRT  string
	}{
		{"embeddings", "/v1/embeddings", map[string]any{"model": "text-embedding-3-small", "input": "hi"}, HandleEmbeddings, "embeddings"},
		{"images", "/v1/images/generations", map[string]any{"model": "dall-e-3", "prompt": "cat"}, HandleImagesGenerations, "images"},
		{"audio", "/v1/audio/speech", map[string]any{"model": "tts-1", "input": "hi"}, HandleAudioSpeech, "audio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			userID := uuid.New()
			apiKeyID := uuid.New()
			executor := &mockExecutor{
				executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
					return &gw.ExecuteResponse{StatusCode: http.StatusBadGateway, Body: map[string]any{"error": map[string]any{"message": "provider down"}}}, nil
				},
				executeRawFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.RawResponse, error) {
					return &gw.RawResponse{StatusCode: http.StatusBadGateway, ContentType: "application/json", Body: []byte(`{"error":{"message":"provider down"}}`)}, nil
				},
			}
			application, walletRepo, usageRepo := newEndpointEnv(executor)

			req := newEndpointRequest(userID, apiKeyID, tc.path, tc.body)
			w := httptest.NewRecorder()
			tc.handler(application).ServeHTTP(w, req)

			if w.Code != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502", w.Code)
			}
			if walletRepo.releaseCalled == 0 {
				t.Error("release should have been called on upstream HTTP error")
			}
			log := waitForUsageLog(t, usageRepo)
			if log.Status != domain.UsageLogStatusFailed {
				t.Errorf("Status = %s, want failed", log.Status)
			}
			if log.ErrorCode != "upstream_http_error" {
				t.Errorf("ErrorCode = %s, want upstream_http_error", log.ErrorCode)
			}
			if log.RequestType != tc.wantRT {
				t.Errorf("RequestType = %q, want %q", log.RequestType, tc.wantRT)
			}
			if usageRepo.lastEvidence == nil || usageRepo.lastEvidence.StatusCode != http.StatusBadGateway {
				t.Errorf("evidence = %+v, want status 502", usageRepo.lastEvidence)
			}
		})
	}
}

func TestHandleForwardedEndpoint_NoWallet_Returns402(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{executeEndpointFn: func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
		return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: map[string]any{}}, nil
	}}
	application, walletRepo, _ := newEndpointEnv(executor)
	// Simulate missing wallet.
	walletRepo.findByUserFn = func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
		return nil, nil
	}

	req := newEndpointRequest(userID, apiKeyID, "/v1/embeddings", map[string]any{"model": "text-embedding-3-small", "input": "hi"})
	w := httptest.NewRecorder()
	HandleEmbeddings(application).ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", w.Code)
	}
	if executor.executeEndpointCalled != 0 {
		t.Error("upstream must not be called when the wallet is missing")
	}
}

func TestHandleForwardedEndpoint_UnknownModel_Returns404(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{}
	application, _, _ := newEndpointEnv(executor)
	application.Router = gw.NewRouter(&mockModelRepo{}, &mockChannelRepo{})

	req := newEndpointRequest(userID, apiKeyID, "/v1/embeddings", map[string]any{"model": "nope", "input": "hi"})
	w := httptest.NewRecorder()
	HandleEmbeddings(application).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleForwardedEndpoint_MissingModel_Returns400(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	application, _, _ := newEndpointEnv(&mockExecutor{})

	req := newEndpointRequest(userID, apiKeyID, "/v1/embeddings", map[string]any{"input": "hi"})
	w := httptest.NewRecorder()
	HandleEmbeddings(application).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleImagesGenerations_InvalidImageCount_Returns400(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	application, _, _ := newEndpointEnv(&mockExecutor{})

	req := newEndpointRequest(userID, apiKeyID, "/v1/images/generations", map[string]any{"model": "dall-e-3", "prompt": "cat", "n": float64(11)})
	w := httptest.NewRecorder()
	HandleImagesGenerations(application).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (n>10 must be rejected)", w.Code)
	}
}

func TestHandleImagesGenerations_NonIntegerCount_Returns400(t *testing.T) {
	for _, n := range []any{1.5, "2", true} {
		t.Run(fmt.Sprintf("%v", n), func(t *testing.T) {
			userID := uuid.New()
			apiKeyID := uuid.New()
			application, _, _ := newEndpointEnv(&mockExecutor{})

			req := newEndpointRequest(userID, apiKeyID, "/v1/images/generations", map[string]any{
				"model":  "dall-e-3",
				"prompt": "cat",
				"n":      n,
			})
			w := httptest.NewRecorder()
			HandleImagesGenerations(application).ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("n=%v: status = %d, want 400 (must not truncate or default)", n, w.Code)
			}
		})
	}
}

func TestHandleForwardedEndpoint_MethodNotAllowed_Returns405(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	application, _, _ := newEndpointEnv(&mockExecutor{})

	req := httptest.NewRequest(http.MethodGet, "/v1/embeddings", nil)
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()
	HandleEmbeddings(application).ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", w.Code)
	}
}

var _ = fmt.Sprintf
var _ = time.Now

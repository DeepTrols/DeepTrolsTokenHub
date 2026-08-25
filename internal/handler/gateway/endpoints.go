package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/provider"
	"github.com/deeptrols/api/internal/service/billing"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HandleEmbeddings implements POST /v1/embeddings: token-based billing with
// the same reserve-before-upstream flow as chat completions.
func HandleEmbeddings(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleForwardedEndpoint(w, r, application, "embeddings", "embeddings",
			validateEmbeddingsRequest, estimateEmbeddingsUsage)
	}
}

// HandleImagesGenerations implements POST /v1/images/generations: billed per
// generated image (request_type "images", pricing dimension "image").
func HandleImagesGenerations(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleForwardedEndpoint(w, r, application, "images/generations", "images",
			validateImagesRequest, estimateImagesUsage)
	}
}

// HandleAudioSpeech implements POST /v1/audio/speech: billed per TTS
// character (request_type "audio", pricing dimension "tts").
func HandleAudioSpeech(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleForwardedRawEndpoint(w, r, application, "audio/speech", "audio",
			validateAudioSpeechRequest, estimateTTSUsage)
	}
}

// HandleAudioTranscriptions implements POST /v1/audio/transcriptions: a
// multipart request (audio file + model) forwarded to the upstream; billed
// per audio second (request_type "audio", pricing dimension "audio").
func HandleAudioTranscriptions(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fields, files, modelName, err := parseMultipartForwardedRequest(w, r, application, maxMultipartBytes)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		handleForwardedMultipartExecution(w, r, application, "audio/transcriptions", "audio",
			modelName, fields, files, estimateSTTUsage)
	}
}

// HandleImagesEdits implements POST /v1/images/edits: a multipart request
// (image file + optional prompt/mask) forwarded to the upstream; billed per
// edited image (request_type "images", pricing dimension "image").
func HandleImagesEdits(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fields, files, modelName, err := parseMultipartForwardedRequest(w, r, application, maxMultipartBytes)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		handleForwardedMultipartExecution(w, r, application, "images/edits", "images",
			modelName, fields, files, estimateImagesEditsUsage)
	}
}

// handleForwardedEndpoint is the shared OpenAI-compatible forwarding path for
// non-chat endpoints. It performs method/body/model validation, API key
// boundary enforcement, routing, budget reserve (before upstream), failover
// execution, settlement, spend tracking, and evidence logging — mirroring the
// non-streaming chat flow so every endpoint leaves the same audit trail.
func handleForwardedEndpoint(
	w http.ResponseWriter,
	r *http.Request,
	application *app.App,
	endpoint, requestType string,
	validate func(body map[string]any) error,
	estimate func(body map[string]any) *usageparser.NormalizedUsage,
) {
	body, modelName, ok := decodeForwardedRequest(w, r, application, validate, estimate)
	if !ok {
		return
	}

	handleForwardedEndpointExecution(w, r, application, endpoint, requestType, modelName, body, estimate)
}

// handleForwardedRawEndpoint mirrors handleForwardedEndpoint for endpoints
// whose upstream response is not JSON (e.g. audio/speech binary audio). The
// request validation and billing pipeline are identical; only the response
// relay differs (bytes + content type instead of JSON).
func handleForwardedRawEndpoint(
	w http.ResponseWriter,
	r *http.Request,
	application *app.App,
	endpoint, requestType string,
	validate func(body map[string]any) error,
	estimate func(body map[string]any) *usageparser.NormalizedUsage,
) {
	body, modelName, ok := decodeForwardedRequest(w, r, application, validate, estimate)
	if !ok {
		return
	}
	handleForwardedRawExecution(w, r, application, endpoint, requestType, modelName, body, estimate)
}

// maxMultipartBytes caps multipart request bodies (audio files / image
// uploads). Matches the common 25 MB upstream limit for transcriptions.
const maxMultipartBytes = 25 << 20

// parseMultipartForwardedRequest validates a multipart request and extracts
// string fields plus file parts. The first file's byte length is recorded as
// the internal "_file_size" field so endpoint estimates can derive duration.
func parseMultipartForwardedRequest(w http.ResponseWriter, r *http.Request, _ *app.App, maxBytes int64) (map[string]any, map[string]gw.MultipartFile, string, error) {
	if r.Method != http.MethodPost {
		return nil, nil, "", fmt.Errorf("only POST is allowed")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		return nil, nil, "", fmt.Errorf("failed to parse multipart form")
	}

	fields := map[string]any{}
	for k, vs := range r.MultipartForm.Value {
		if len(vs) > 0 {
			fields[k] = vs[len(vs)-1]
		}
	}
	files := map[string]gw.MultipartFile{}
	for name, fhs := range r.MultipartForm.File {
		if len(fhs) == 0 {
			continue
		}
		fh := fhs[0]
		f, err := fh.Open()
		if err != nil {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(f, maxBytes))
		_ = f.Close()
		if err != nil {
			continue
		}
		files[name] = gw.MultipartFile{
			FileName:    fh.Filename,
			ContentType: fh.Header.Get("Content-Type"),
			Content:     content,
		}
		if _, ok := fields["_file_size"]; !ok {
			fields["_file_size"] = int64(len(content))
		}
	}

	modelName, _ := fields["model"].(string)
	if modelName == "" {
		return nil, nil, "", fmt.Errorf("model is required")
	}
	if len(files) == 0 {
		return nil, nil, "", fmt.Errorf("file is required")
	}
	return fields, files, modelName, nil
}

// estimateSTTUsage derives an audio-seconds estimate from the uploaded file
// size (128 kbps ≈ 16 KB/s). The upstream rarely returns usage for
// transcriptions, so this estimate is the billing basis.
func estimateSTTUsage(fields map[string]any) *usageparser.NormalizedUsage {
	size, _ := fields["_file_size"].(int64)
	seconds := size / 16_000
	if seconds <= 0 {
		seconds = 1
	}
	return &usageparser.NormalizedUsage{AudioSeconds: seconds}
}

// estimateImagesEditsUsage bills the requested edit count (default 1).
func estimateImagesEditsUsage(fields map[string]any) *usageparser.NormalizedUsage {
	n := int64(1)
	if raw, ok := fields["n"].(string); ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil && parsed > 0 {
			n = parsed
		}
	}
	return &usageparser.NormalizedUsage{ImageCount: n}
}

// handleForwardedMultipartExecution runs the reserve → execute → settle →
// log pipeline for multipart endpoints (audio/transcriptions, images/edits),
// mirroring the raw execution flow with failover across routed candidates.
func handleForwardedMultipartExecution(
	w http.ResponseWriter,
	r *http.Request,
	application *app.App,
	endpoint, requestType, modelName string,
	fields map[string]any,
	files map[string]gw.MultipartFile,
	estimate func(fields map[string]any) *usageparser.NormalizedUsage,
) {
	cfg := application.Config
	userID, apiKeyID := resolveIdentity(r)

	candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
	if err != nil {
		log.Printf("gateway: %s route error: %v", endpoint, err)
		writeRouteError(w, err)
		return
	}
	if len(candidates) == 0 {
		writeRouteError(w, gw.ErrNoChannelAvailable)
		return
	}
	primary := candidates[0]

	var tenantID *uuid.UUID
	if identity := resolveAuthIdentity(r); identity != nil {
		tenantID = identity.TenantID
	}

	// Budget reservation must precede the upstream call (invariant #2).
	estimatedUsage := estimate(fields)
	priceResult, err := application.Pricer.Calculate(r.Context(), primary.Channel.ModelID, tenantID, estimatedUsage)
	holdAmount := decimal.Zero
	if err != nil {
		log.Printf("gateway: %s pricer estimate error: %v (using minimum hold)", endpoint, err)
		holdAmount, _ = decimal.NewFromString(minHoldAmount)
	} else {
		holdAmount = priceResult.ListCost
		if rejectIncompletePricing(w, priceResult) {
			return
		}
	}
	if holdAmount.LessThanOrEqual(decimal.Zero) {
		holdAmount, _ = decimal.NewFromString(minHoldAmount)
	}

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}

	// Tenant budget gate (Phase 1): check estimated cost before upstream.

	wallet, err := application.Wallets.FindByUser(r.Context(), userID, nil)
	if err != nil {
		log.Printf("gateway: %s wallet lookup error: %v", endpoint, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to verify account")
		return
	}
	if wallet == nil {
		writeError(w, http.StatusPaymentRequired, "wallet_missing", "No wallet for this account")
		return
	}

	executor := application.Executor
	if executor == nil {
		executor = provider.NewOpenAICompatAdapter()
	}

	startTime := time.Now()
	var (
		reserveResult     *billing.ReserveResult
		resp              *gw.ExecuteResponse
		lastErr           error
		upstreamFailed    = true
		lastResp          *gw.ExecuteResponse
		lastRouteResult   *gw.RouteResult
		upstreamModelName = modelName
		attemptCount      int
	)
	for i := range candidates {
		cand := candidates[i]
		attemptID := requestID
		if i > 0 {
			attemptID = fmt.Sprintf("%s-a%d", requestID, i)
		}

		baseURL := stringOrDefault(cand.Instance.BaseURL, cfg.LiteLLM.BaseURL)
		apiKey := cfg.LiteLLM.MasterKey
		if key, ok := cand.Instance.Config["api_key"].(string); ok && key != "" {
			apiKey = key
		}
		upstreamModel := stringOrDefault(cand.UpstreamModel, modelName)

		var loadHold *gw.LoadHold
		if application.LoadTracker != nil {
			loadHold, _ = application.LoadTracker.Acquire(r.Context(), cand.Instance.ID)
		}
		if !wallet.CanReserve(holdAmount) {
			writeError(w, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance")
			if loadHold != nil {
				loadHold.Release()
			}
			return
		}
		rr, rerr := application.Charger.Reserve(r.Context(), wallet.ID, holdAmount, attemptID)
		if rerr != nil {
			log.Printf("gateway: %s reserve error: %v", endpoint, rerr)
			writeError(w, http.StatusInternalServerError, "internal_error", "Service temporarily unavailable")
			if loadHold != nil {
				loadHold.Release()
			}
			return
		}
		reserveResult = rr

		resp, lastErr = executor.ExecuteEndpointMultipart(r.Context(), baseURL, apiKey, upstreamModel, endpoint, fields, files,
			gw.CustomHeadersFromConfig(cand.Instance.Config))
		if loadHold != nil {
			loadHold.Release()
		}
		upstreamFailed = lastErr != nil || (resp != nil && resp.StatusCode >= 400)
		if !upstreamFailed {
			lastRouteResult = &cand
			break
		}

		lastResp = resp
		lastRouteResult = &cand
		attemptCount = i + 1
		upstreamModelName = stringOrDefault(cand.UpstreamModel, modelName)
		releaseWalletHold(r.Context(), application, reserveResult.TransactionID)
		reserveResult = nil
		log.Printf("gateway: %s attempt %d failed channel=%q: %v", endpoint, i, cand.Channel.Name, lastErr)
	}

	if upstreamFailed {
		msg := "Upstream request failed"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		synthetic := (*gw.ExecuteResponse)(nil)
		if lastResp != nil {
			synthetic = lastResp
		}
		settleMinuteBucket(r, application, 0)
		log.Printf("gateway: all %s attempts failed: %v", endpoint, lastErr)
		go logNonStreamFailure(application, requestType, userID, apiKeyID, tenantID, modelName, upstreamModelName,
			lastRouteResult, fields, requestID, lastErr, synthetic, attemptCount, int(time.Since(startTime).Milliseconds()))
		if reserveResult != nil {
			releaseWalletHold(r.Context(), application, reserveResult.TransactionID)
		}
		writeError(w, http.StatusBadGateway, "upstream_error", msg)
		return
	}
	routeResult := lastRouteResult

	// Actual usage: prefer upstream usage, fall back to the request estimate.
	actualUsage := resp.Usage
	if actualUsage == nil || !actualUsage.HasUsage() {
		actualUsage = estimate(fields)
		resp.Usage = actualUsage
		resp.UsageSource = usageparser.SourceEstimated
	}
	settleMinuteBucket(r, application, actualUsage.TotalTokens)

	actualCosts := calculateActualCosts(r.Context(), application, routeResult, resp, tenantID)
	finalCost := decimal.Zero
	if actualCosts != nil {
		finalCost = actualCosts.ListCost
	}
	walletCharged := finalCost
	underfunded := false
	pricingIncomplete := actualCosts != nil && len(actualCosts.MissingPricing) > 0
	if pricingIncomplete {
		log.Printf("gateway: %s pricing incomplete for dims %v; charging reserved hold %s", endpoint, actualCosts.MissingPricing, holdAmount)
		finalCost = holdAmount
		walletCharged = holdAmount
	}
	if settleErr := application.Charger.Settle(r.Context(), reserveResult.TransactionID, finalCost); settleErr != nil {
		log.Printf("gateway: %s settle error tx=%s final=%s: %v (falling back to reserved commit)", endpoint, reserveResult.TransactionID, finalCost, settleErr)
		if commitErr := application.Charger.Commit(r.Context(), reserveResult.TransactionID); commitErr != nil {
			log.Printf("gateway: %s commit error tx=%s: %v", endpoint, reserveResult.TransactionID, commitErr)
		}
		walletCharged = holdAmount
		underfunded = true
	}
	if actualCosts != nil {
		recordAPIKeySpend(r.Context(), application, apiKeyID, actualCosts.ListCost)
	}

	upstreamModel := stringOrDefault(routeResult.UpstreamModel, modelName)
	go logUsageWithCosts(r, application, requestType, userID, apiKeyID, modelName, upstreamModel, resp, routeResult, actualCosts, walletCharged, underfunded, pricingIncomplete)

	// Relay: plain-text transcription responses come back wrapped as {"text": ...}.
	if text, ok := resp.Body["text"].(string); ok && len(resp.Body) == 1 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		if _, err := w.Write([]byte(text)); err != nil {
			log.Printf("gateway: %s failed to relay text response: %v", endpoint, err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if err := json.NewEncoder(w).Encode(resp.Body); err != nil {
		log.Printf("gateway: %s failed to relay JSON response: %v", endpoint, err)
	}
}

// decodeForwardedRequest performs the shared method/body/model validation and
// strips client fields that could override upstream routing or credentials.
func decodeForwardedRequest(w http.ResponseWriter, r *http.Request, application *app.App, validate func(body map[string]any) error, estimate func(body map[string]any) *usageparser.NormalizedUsage) (map[string]any, string, bool) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
		return nil, "", false
	}

	var body map[string]any
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return nil, "", false
	}

	modelName, _ := body["model"].(string)
	if modelName == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "model is required")
		return nil, "", false
	}
	if err := validate(body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return nil, "", false
	}
	// Strip fields that could override upstream routing/security params.
	sanitizeRequestBody(body)

	// Enforce API key governance boundaries before routing/billing so the
	// model allowlist, IP whitelist, and spend limits apply to every endpoint.
	if err := enforceAPIKeyBoundaries(w, r, application, modelName, estimate(body).TotalTokens); err != nil {
		var be *boundaryError
		if errors.As(err, &be) {
			writeError(w, be.status, be.errType, be.message)
			return nil, "", false
		}
		log.Printf("gateway: boundary check error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to verify API key policy")
		return nil, "", false
	}
	return body, modelName, true
}

// handleForwardedRawExecution runs the same reserve → execute → settle → log
// pipeline as handleForwardedEndpointExecution, but for endpoints whose
// upstream response is not JSON (e.g. audio/speech binary audio). The binary
// body is relayed to the client with the upstream content type; evidence
// stores only metadata (content type + byte length), never the audio bytes.
func handleForwardedRawExecution(
	w http.ResponseWriter,
	r *http.Request,
	application *app.App,
	endpoint, requestType, modelName string,
	body map[string]any,
	estimate func(body map[string]any) *usageparser.NormalizedUsage,
) {
	cfg := application.Config
	userID, apiKeyID := resolveIdentity(r)

	candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
	if err != nil {
		log.Printf("gateway: %s route error: %v", endpoint, err)
		writeRouteError(w, err)
		return
	}
	if len(candidates) == 0 {
		writeRouteError(w, gw.ErrNoChannelAvailable)
		return
	}
	primary := candidates[0]

	var tenantID *uuid.UUID
	if identity := resolveAuthIdentity(r); identity != nil {
		tenantID = identity.TenantID
	}

	// Budget reservation must precede the upstream call (invariant #2).
	estimatedUsage := estimate(body)
	priceResult, err := application.Pricer.Calculate(r.Context(), primary.Channel.ModelID, tenantID, estimatedUsage)
	holdAmount := decimal.Zero
	if err != nil {
		log.Printf("gateway: %s pricer estimate error: %v (using minimum hold)", endpoint, err)
		holdAmount, _ = decimal.NewFromString(minHoldAmount)
	} else {
		holdAmount = priceResult.ListCost
		if rejectIncompletePricing(w, priceResult) {
			return
		}
	}
	if holdAmount.LessThanOrEqual(decimal.Zero) {
		holdAmount, _ = decimal.NewFromString(minHoldAmount)
	}

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}

	// Tenant budget gate (Phase 1): check estimated cost before upstream.

	wallet, err := application.Wallets.FindByUser(r.Context(), userID, nil)
	if err != nil {
		log.Printf("gateway: %s wallet lookup error: %v", endpoint, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to verify account")
		return
	}
	if wallet == nil {
		writeError(w, http.StatusPaymentRequired, "wallet_missing", "No wallet for this account")
		return
	}

	executor := application.Executor
	if executor == nil {
		executor = provider.NewOpenAICompatAdapter()
	}

	startTime := time.Now()
	var (
		reserveResult     *billing.ReserveResult
		raw               *gw.RawResponse
		lastErr           error
		upstreamFailed    = true
		lastRaw           *gw.RawResponse
		lastRouteResult   *gw.RouteResult
		upstreamModelName = modelName
		attemptCount      int
	)
	for i := range candidates {
		cand := candidates[i]
		attemptID := requestID
		if i > 0 {
			attemptID = fmt.Sprintf("%s-a%d", requestID, i)
		}

		baseURL := stringOrDefault(cand.Instance.BaseURL, cfg.LiteLLM.BaseURL)
		apiKey := cfg.LiteLLM.MasterKey
		if key, ok := cand.Instance.Config["api_key"].(string); ok && key != "" {
			apiKey = key
		}
		upstreamModel := stringOrDefault(cand.UpstreamModel, modelName)

		// Track real-time in-flight load for this instance (best-effort).
		var loadHold *gw.LoadHold
		if application.LoadTracker != nil {
			loadHold, _ = application.LoadTracker.Acquire(r.Context(), cand.Instance.ID)
		}
		if !wallet.CanReserve(holdAmount) {
			writeError(w, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance")
			if loadHold != nil {
				loadHold.Release()
			}
			return
		}
		rr, rerr := application.Charger.Reserve(r.Context(), wallet.ID, holdAmount, attemptID)
		if rerr != nil {
			log.Printf("gateway: %s reserve error: %v", endpoint, rerr)
			writeError(w, http.StatusInternalServerError, "internal_error", "Service temporarily unavailable")
			if loadHold != nil {
				loadHold.Release()
			}
			return
		}
		reserveResult = rr

		raw, lastErr = executor.ExecuteEndpointRaw(r.Context(), baseURL, apiKey, upstreamModel, endpoint, body,
			gw.CustomHeadersFromConfig(cand.Instance.Config))
		if loadHold != nil {
			loadHold.Release()
		}
		upstreamFailed = lastErr != nil || (raw != nil && raw.StatusCode >= 400)
		if !upstreamFailed {
			lastRouteResult = &cand
			break
		}

		lastRaw = raw
		lastRouteResult = &cand
		attemptCount = i + 1
		upstreamModelName = stringOrDefault(cand.UpstreamModel, modelName)
		releaseWalletHold(r.Context(), application, reserveResult.TransactionID)
		reserveResult = nil
		log.Printf("gateway: %s attempt %d failed channel=%q: %v", endpoint, i, cand.Channel.Name, lastErr)
	}

	if upstreamFailed {
		// Build a synthetic JSON response for the shared failure logger so
		// error classification and evidence capture stay identical.
		synthetic := (*gw.ExecuteResponse)(nil)
		if lastRaw != nil {
			errBody := map[string]any{}
			if json.Unmarshal(lastRaw.Body, &errBody) != nil {
				if len(lastRaw.Body) > maxUpstreamErrorBody {
					errBody = map[string]any{"raw": string(lastRaw.Body[:maxUpstreamErrorBody])}
				} else {
					errBody = map[string]any{"raw": string(lastRaw.Body)}
				}
			}
			synthetic = &gw.ExecuteResponse{StatusCode: lastRaw.StatusCode, Body: errBody}
		}
		msg := "Upstream request failed"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		settleMinuteBucket(r, application, 0)
		log.Printf("gateway: all %s attempts failed: %v", endpoint, lastErr)
		go logNonStreamFailure(application, requestType, userID, apiKeyID, tenantID, modelName, upstreamModelName,
			lastRouteResult, body, requestID, lastErr, synthetic, attemptCount, int(time.Since(startTime).Milliseconds()))
		writeError(w, http.StatusBadGateway, "upstream_error", msg)
		return
	}
	routeResult := lastRouteResult

	// Actual usage: upstream rarely reports usage for raw endpoints, so bill
	// the request-derived estimate (TTS characters).
	actualUsage := estimate(body)
	settleMinuteBucket(r, application, actualUsage.TotalTokens)

	synthetic := &gw.ExecuteResponse{
		StatusCode:    raw.StatusCode,
		Usage:         actualUsage,
		UsageSource:   usageparser.SourceEstimated,
		ProviderReqID: raw.ProviderReqID,
		DurationMs:    raw.DurationMs,
		Body:          map[string]any{"content_type": raw.ContentType, "bytes": len(raw.Body)},
	}

	actualCosts := calculateActualCosts(r.Context(), application, routeResult, synthetic, tenantID)
	finalCost := decimal.Zero
	if actualCosts != nil {
		finalCost = actualCosts.ListCost
	}
	walletCharged := finalCost
	underfunded := false
	pricingIncomplete := actualCosts != nil && len(actualCosts.MissingPricing) > 0
	if pricingIncomplete {
		// Never let a misconfigured price produce a free call: charge the
		// reserved hold and record the evidence for reconciliation.
		log.Printf("gateway: %s pricing incomplete for dims %v; charging reserved hold %s", endpoint, actualCosts.MissingPricing, holdAmount)
		finalCost = holdAmount
		walletCharged = holdAmount
	}
	if settleErr := application.Charger.Settle(r.Context(), reserveResult.TransactionID, finalCost); settleErr != nil {
		// Commit the reserved amount and RECORD the shortfall in the
		// evidence chain; never disguise an undercharge as full settlement.
		log.Printf("gateway: %s settle error tx=%s final=%s: %v (falling back to reserved commit)", endpoint, reserveResult.TransactionID, finalCost, settleErr)
		if commitErr := application.Charger.Commit(r.Context(), reserveResult.TransactionID); commitErr != nil {
			log.Printf("gateway: %s commit error tx=%s: %v", endpoint, reserveResult.TransactionID, commitErr)
		}
		walletCharged = holdAmount
		underfunded = true
	}
	if actualCosts != nil {
		recordAPIKeySpend(r.Context(), application, apiKeyID, actualCosts.ListCost)
	}

	upstreamModel := stringOrDefault(routeResult.UpstreamModel, modelName)
	go logUsageWithCosts(r, application, requestType, userID, apiKeyID, modelName, upstreamModel, synthetic, routeResult, actualCosts, walletCharged, underfunded, pricingIncomplete)

	if raw.ContentType != "" {
		w.Header().Set("Content-Type", raw.ContentType)
	}
	w.WriteHeader(raw.StatusCode)
	if _, err := w.Write(raw.Body); err != nil {
		log.Printf("gateway: %s failed to relay response: %v", endpoint, err)
	}
}

// handleForwardedEndpointExecution runs the reserve → execute → settle →
// log pipeline for a forwarded endpoint. The flow intentionally mirrors
// HandleNonStreamingChat: failover across routed candidates, per-attempt
// idempotent wallet holds, and a detached failure log when all candidates
// fail (invariant #4 — no request may vanish from the evidence chain).
func handleForwardedEndpointExecution(
	w http.ResponseWriter,
	r *http.Request,
	application *app.App,
	endpoint, requestType, modelName string,
	body map[string]any,
	estimate func(body map[string]any) *usageparser.NormalizedUsage,
) {
	cfg := application.Config
	userID, apiKeyID := resolveIdentity(r)

	candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
	if err != nil {
		log.Printf("gateway: %s route error: %v", endpoint, err)
		writeRouteError(w, err)
		return
	}
	if len(candidates) == 0 {
		writeRouteError(w, gw.ErrNoChannelAvailable)
		return
	}
	primary := candidates[0]

	var tenantID *uuid.UUID
	if identity := resolveAuthIdentity(r); identity != nil {
		tenantID = identity.TenantID
	}

	// Estimate usage and compute the budget hold BEFORE the upstream call
	// (invariant #2 — budget reservation precedes upstream invocation).
	estimatedUsage := estimate(body)
	priceResult, err := application.Pricer.Calculate(r.Context(), primary.Channel.ModelID, tenantID, estimatedUsage)
	holdAmount := decimal.Zero
	if err != nil {
		log.Printf("gateway: %s pricer estimate error: %v (using minimum hold)", endpoint, err)
		holdAmount, _ = decimal.NewFromString(minHoldAmount)
	} else {
		holdAmount = priceResult.ListCost
		if rejectIncompletePricing(w, priceResult) {
			return
		}
	}
	if holdAmount.LessThanOrEqual(decimal.Zero) {
		holdAmount, _ = decimal.NewFromString(minHoldAmount)
	}

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}

	// Tenant budget gate (Phase 1): check estimated cost before upstream.

	wallet, err := application.Wallets.FindByUser(r.Context(), userID, nil)
	if err != nil {
		log.Printf("gateway: %s wallet lookup error: %v", endpoint, err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to verify account")
		return
	}
	if wallet == nil {
		// Fail-closed: every calling account must have a wallet to hold budget.
		writeError(w, http.StatusPaymentRequired, "wallet_missing", "No wallet for this account")
		return
	}

	executor := application.Executor
	if executor == nil {
		executor = provider.NewOpenAICompatAdapter()
	}

	startTime := time.Now()
	var (
		reserveResult     *billing.ReserveResult
		resp              *gw.ExecuteResponse
		upstreamFailed    = true
		lastErr           error
		lastResp          *gw.ExecuteResponse
		lastRouteResult   *gw.RouteResult
		upstreamModelName = modelName
		attemptCount      int
	)
	for i := range candidates {
		cand := candidates[i]
		attemptID := requestID
		if i > 0 {
			attemptID = fmt.Sprintf("%s-a%d", requestID, i)
		}

		baseURL := stringOrDefault(cand.Instance.BaseURL, cfg.LiteLLM.BaseURL)
		apiKey := cfg.LiteLLM.MasterKey
		if key, ok := cand.Instance.Config["api_key"].(string); ok && key != "" {
			apiKey = key
		}
		upstreamModel := stringOrDefault(cand.UpstreamModel, modelName)

		// Track real-time in-flight load for this instance (best-effort).
		var loadHold *gw.LoadHold
		if application.LoadTracker != nil {
			loadHold, _ = application.LoadTracker.Acquire(r.Context(), cand.Instance.ID)
		}
		if !wallet.CanReserve(holdAmount) {
			writeError(w, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance")
			if loadHold != nil {
				loadHold.Release()
			}
			return
		}
		rr, rerr := application.Charger.Reserve(r.Context(), wallet.ID, holdAmount, attemptID)
		if rerr != nil {
			log.Printf("gateway: %s reserve error: %v", endpoint, rerr)
			writeError(w, http.StatusInternalServerError, "internal_error", "Service temporarily unavailable")
			if loadHold != nil {
				loadHold.Release()
			}
			return
		}
		reserveResult = rr

		resp, lastErr = executor.ExecuteEndpoint(r.Context(), baseURL, apiKey, upstreamModel, endpoint, body,
			gw.CustomHeadersFromConfig(cand.Instance.Config))
		if loadHold != nil {
			loadHold.Release()
		}
		upstreamFailed = lastErr != nil || (resp != nil && resp.StatusCode >= 400)
		if !upstreamFailed {
			lastRouteResult = &cand
			break
		}

		// Attempt failed: remember evidence, release this hold (with a detached
		// fallback so a client disconnect can never freeze the hold), and try
		// the next candidate.
		lastResp = resp
		lastRouteResult = &cand
		attemptCount = i + 1
		upstreamModelName = stringOrDefault(cand.UpstreamModel, modelName)
		releaseWalletHold(r.Context(), application, reserveResult.TransactionID)
		reserveResult = nil
		log.Printf("gateway: %s attempt %d failed channel=%q: %v", endpoint, i, cand.Channel.Name, lastErr)
	}

	if upstreamFailed {
		msg := "Upstream request failed"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		settleMinuteBucket(r, application, 0)
		log.Printf("gateway: all %s attempts failed: %v", endpoint, lastErr)
		go logNonStreamFailure(application, requestType, userID, apiKeyID, tenantID, modelName, upstreamModelName,
			lastRouteResult, body, requestID, lastErr, lastResp, attemptCount, int(time.Since(startTime).Milliseconds()))
		writeError(w, http.StatusBadGateway, "upstream_error", msg)
		return
	}
	routeResult := lastRouteResult

	// Actual usage: prefer upstream usage; for image/TTS endpoints (which
	// rarely report usage) fall back to the request-derived estimate.
	actualUsage := resp.Usage
	if actualUsage == nil || !actualUsage.HasUsage() {
		actualUsage = estimate(body)
		resp.Usage = actualUsage
	}
	settleMinuteBucket(r, application, actualUsage.TotalTokens)

	// Settle reserved funds against the REAL final cost.
	actualCosts := calculateActualCosts(r.Context(), application, routeResult, resp, tenantID)
	finalCost := decimal.Zero
	if actualCosts != nil {
		finalCost = actualCosts.ListCost
	}
	walletCharged := finalCost
	underfunded := false
	pricingIncomplete := actualCosts != nil && len(actualCosts.MissingPricing) > 0
	if pricingIncomplete {
		// Never let a misconfigured price produce a free call: charge the
		// reserved hold and record the evidence for reconciliation.
		log.Printf("gateway: %s pricing incomplete for dims %v; charging reserved hold %s", endpoint, actualCosts.MissingPricing, holdAmount)
		finalCost = holdAmount
		walletCharged = holdAmount
	}
	if settleErr := application.Charger.Settle(r.Context(), reserveResult.TransactionID, finalCost); settleErr != nil {
		// Commit the reserved amount and RECORD the shortfall in the
		// evidence chain; never disguise an undercharge as full settlement.
		log.Printf("gateway: %s settle error tx=%s final=%s: %v (falling back to reserved commit)", endpoint, reserveResult.TransactionID, finalCost, settleErr)
		if commitErr := application.Charger.Commit(r.Context(), reserveResult.TransactionID); commitErr != nil {
			log.Printf("gateway: %s commit error tx=%s: %v", endpoint, reserveResult.TransactionID, commitErr)
		}
		walletCharged = holdAmount
		underfunded = true
	}
	// Record spend against API key limits (best-effort, after settle).
	if actualCosts != nil {
		recordAPIKeySpend(r.Context(), application, apiKeyID, actualCosts.ListCost)
	}

	// Log usage in background with a detached context so it survives the HTTP
	// request lifecycle.
	upstreamModel := stringOrDefault(routeResult.UpstreamModel, modelName)
	go logUsageWithCosts(r, application, requestType, userID, apiKeyID, modelName, upstreamModel, resp, routeResult, actualCosts, walletCharged, underfunded, pricingIncomplete)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if err := json.NewEncoder(w).Encode(resp.Body); err != nil {
		log.Printf("gateway: %s failed to encode response: %v", endpoint, err)
	}
}

// validateEmbeddingsRequest requires a non-empty input (string or array).
func validateEmbeddingsRequest(body map[string]any) error {
	switch v := body["input"].(type) {
	case string:
		if v == "" {
			return fmt.Errorf("input is required")
		}
	case []any:
		if len(v) == 0 {
			return fmt.Errorf("input is required")
		}
	default:
		return fmt.Errorf("input is required")
	}
	return nil
}

// estimateEmbeddingsUsage estimates prompt tokens from the input length.
func estimateEmbeddingsUsage(body map[string]any) *usageparser.NormalizedUsage {
	var tokens int64
	switch v := body["input"].(type) {
	case string:
		tokens = usageparser.EstimateTextTokens(v)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				tokens += usageparser.EstimateTextTokens(s)
			}
		}
	}
	if tokens <= 0 {
		tokens = 1
	}
	return &usageparser.NormalizedUsage{InputTokens: tokens, TotalTokens: tokens}
}

// validateImagesRequest requires an image count within 1..10. The prompt is
// validated upstream (OpenAI-compatible providers return 400 when missing).
func validateImagesRequest(body map[string]any) error {
	n, err := imageCountFromBody(body)
	if err != nil {
		return err
	}
	if n < 1 || n > 10 {
		return fmt.Errorf("n must be between 1 and 10")
	}
	return nil
}

// imageCountFromBody returns the requested image count (default 1). Only
// integers are accepted: fractional counts (n=1.5) and non-numeric values
// (n="2") are rejected instead of being silently truncated or defaulted.
func imageCountFromBody(body map[string]any) (int, error) {
	switch v := body["n"].(type) {
	case nil:
		return 1, nil
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("n must be an integer")
		}
		return int(v), nil
	case json.Number:
		if iv, err := v.Int64(); err == nil {
			return int(iv), nil
		}
		return 0, fmt.Errorf("n must be an integer")
	default:
		return 0, fmt.Errorf("n must be an integer")
	}
}

// estimateImagesUsage bills the requested image count.
func estimateImagesUsage(body map[string]any) *usageparser.NormalizedUsage {
	n, err := imageCountFromBody(body)
	if err != nil || n < 1 {
		n = 1
	}
	return &usageparser.NormalizedUsage{ImageCount: int64(n)}
}

// validateAudioSpeechRequest requires a non-empty input string.
func validateAudioSpeechRequest(body map[string]any) error {
	input, _ := body["input"].(string)
	if input == "" {
		return fmt.Errorf("input is required")
	}
	return nil
}

// estimateTTSUsage bills the character count of the TTS input.
func estimateTTSUsage(body map[string]any) *usageparser.NormalizedUsage {
	input, _ := body["input"].(string)
	return &usageparser.NormalizedUsage{TTSCharacters: int64(utf8.RuneCountInString(input))}
}

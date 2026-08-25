package gateway

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/service/billing"
	"github.com/deeptrols/api/internal/service/cache"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	// minHoldAmount is the minimum budget reserve when pricer returns zero.
	minHoldAmount = "0.0001"
	// estimatedOutputTokens is the assumed minimum output tokens for price estimation.
	estimatedOutputTokens = 256
	// charsPerToken is the rough ratio for estimating input tokens from message length.
	charsPerToken = 4
	// maxUpstreamErrorBody caps the upstream error body read back and stored
	// in provider evidence, bounding memory and evidence-table growth.
	maxUpstreamErrorBody = 1 << 20
)

// HandleChatCompletions is the main OpenAI-compatible chat completions endpoint.
func HandleChatCompletions(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
			return
		}

		var body map[string]any
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
			return
		}

		modelName, _ := body["model"].(string)
		if modelName == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "model is required")
			return
		}
		// Strip fields that could override upstream routing/security params.
		sanitizeRequestBody(body)

		// Enforce API key governance boundaries (model allowlist / IP / spend
		// limits) before routing, billing, or cache so every request — cache
		// hits included — is subject to the same policy.
		if err := enforceAPIKeyBoundaries(r, application, modelName); err != nil {
			var be *boundaryError
			if errors.As(err, &be) {
				writeError(w, be.status, be.errType, be.message)
				return
			}
			log.Printf("gateway: boundary check error: %v", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to verify API key policy")
			return
		}

		stream, _ := body["stream"].(bool)
		if stream {
			HandleStreamingChat(w, r, application, modelName, body)

			return
		}

		HandleNonStreamingChat(w, r, application, modelName, body)
	}
}

// HandleNonStreamingChat handles non-streaming chat completion requests with
// budget reserve, upstream execution, and usage logging.
func HandleNonStreamingChat(w http.ResponseWriter, r *http.Request, application *app.App, modelName string, body map[string]any) {
	cfg := application.Config

	// ---- Response cache check (before routing / billing) ----
	cacheSvc := application.ResponseCache
	var cacheKey string
	if cacheSvc != nil && cacheSvc.IsEnabled() && cacheSvc.IsModelAccepted(modelName) {
		cacheKey = cache.BuildKey(modelName, body, cacheScope(r))
		if cached, err := cacheSvc.Get(r.Context(), cacheKey); err == nil && cached != nil {
			var respBody map[string]any
			if err := json.Unmarshal([]byte(cached.Body), &respBody); err == nil {
				respBody["model"] = modelName
				logUsageCacheHit(r, application, modelName, cached)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Cache", "HIT")
				w.WriteHeader(cached.StatusCode)
				json.NewEncoder(w).Encode(respBody)
				return
			}
		}
	}

	// Resolve identity from auth context (set by GatewayAuth middleware).
	userID, apiKeyID := resolveIdentity(r)

	// Route through the router to get an ordered list of candidates; on
	// upstream failure we fail over to the next candidate instead of failing
	// the whole request.
	candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
	if err != nil {
		log.Printf("gateway: route error: %v", err)
		writeRouteError(w, err)
		return
	}
	primary := candidates[0]

	// Resolve tenant ID for pricing.
	identity := resolveAuthIdentity(r)
	var tenantID *uuid.UUID
	if identity != nil {
		tenantID = identity.TenantID
	}

	// Estimate usage from request body, calculate hold amount via pricer.
	estimatedUsage := estimateUsageFromBody(body)
	priceResult, err := application.Pricer.Calculate(r.Context(), primary.Channel.ModelID, tenantID, estimatedUsage)
	holdAmount := decimal.Zero
	if err != nil {
		log.Printf("gateway: pricer estimate error: %v (using minimum hold)", err)
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

	// Budget reserve: lookup wallet and hold funds before upstream call.
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}

	wallet, err := application.Wallets.FindByUser(r.Context(), userID, nil)
	if err != nil {
		log.Printf("gateway: wallet lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to verify account")
		return
	}
	if wallet == nil {
		// Fail-closed: every calling account must have a wallet to hold budget.
		writeError(w, http.StatusPaymentRequired, "wallet_missing", "No wallet for this account")
		return
	}

	// Execute with failover across the ordered candidates. Each attempt uses a
	// distinct idempotency key so wallet holds do not collide on retry.
	executor := application.Executor
	if executor == nil {
		executor = gw.NewLiteLLMExecutor()
	}

	var (
		reserveResult     *billing.ReserveResult
		routeResult       *gw.RouteResult
		resp              *gw.ExecuteResponse
		upstreamFailed    = true
		lastErr           error
		lastResp          *gw.ExecuteResponse
		lastRouteResult   *gw.RouteResult
		upstreamModelName = modelName
		attemptCount      int
	)
	startTime := time.Now()
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

		// Track real-time in-flight load for this instance (best-effort:
		// a Redis hiccup must never fail the request).
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
			log.Printf("gateway: reserve error: %v", rerr)
			writeError(w, http.StatusInternalServerError, "internal_error", "Service temporarily unavailable")
			if loadHold != nil {
				loadHold.Release()
			}
			return
		}
		reserveResult = rr

		resp, lastErr = executor.Execute(r.Context(), baseURL, apiKey, upstreamModel, body)
		if loadHold != nil {
			loadHold.Release()
		}
		upstreamFailed = lastErr != nil || (resp != nil && resp.StatusCode >= 400)
		if !upstreamFailed {
			routeResult = &cand
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
		log.Printf("gateway: attempt %d failed channel=%q: %v", i, cand.Channel.Name, lastErr)
	}

	if upstreamFailed {
		msg := "Upstream request failed"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		log.Printf("gateway: all upstream attempts failed: %v", lastErr)
		// An upstream failure that already consumed tokens (the response
		// carries a usage object, e.g. context_length_exceeded) is real spend
		// and must be billed even though the request failed for the client.
		if lastResp != nil && lastResp.Usage != nil && lastResp.Usage.HasUsage() {
			actualCosts := calculateActualCosts(r.Context(), application, lastRouteResult, lastResp, tenantID)
			finalCost := decimal.Zero
			if actualCosts != nil {
				finalCost = actualCosts.ListCost
			}
			walletCharged := decimal.Zero
			underfunded := false
			if finalCost.GreaterThan(decimal.Zero) {
				// The per-attempt hold was already released; re-reserve the
				// actual cost before settling so the wallet check still runs.
				rr, rerr := application.Charger.Reserve(r.Context(), wallet.ID, finalCost, requestID+"-usage")
				if rerr == nil {
					if sErr := application.Charger.Settle(r.Context(), rr.TransactionID, finalCost); sErr != nil {
						// Commit the reserved amount and record the shortfall
						// in the evidence chain for reconciliation.
						_ = application.Charger.Commit(r.Context(), rr.TransactionID)
						walletCharged = finalCost
						underfunded = true
					} else {
						walletCharged = finalCost
					}
				}
			}
			pricingIncomplete := actualCosts != nil && len(actualCosts.MissingPricing) > 0
			go logUsageWithCosts(r, application, "chat", userID, apiKeyID, modelName, upstreamModelName,
				lastResp, lastRouteResult, actualCosts, walletCharged, underfunded, pricingIncomplete)
			writeError(w, http.StatusBadGateway, "upstream_error", msg)
			return
		}
		// Record the failed call in the evidence chain (invariant #4): every
		// upstream failure leaves a zero-cost failed usage_log so billing
		// reconciliation never sees "missing" requests.
		go logNonStreamFailure(application, "chat", userID, apiKeyID, tenantID, modelName, upstreamModelName,
			lastRouteResult, body, requestID, lastErr, lastResp, attemptCount, int(time.Since(startTime).Milliseconds()))
		writeError(w, http.StatusBadGateway, "upstream_error", msg)
		return
	}

	// Calculate actual costs from upstream response usage (before settling,
	// so the wallet is charged the REAL final cost, not the estimate).
	// A successful upstream response without usage must never settle at zero:
	// fall back to the request-derived estimate and mark it estimated so
	// reconciliation can see the degraded usage source.
	if resp.Usage == nil || !resp.Usage.HasUsage() {
		resp.Usage = estimateUsageFromBody(body)
		resp.UsageSource = usageparser.SourceEstimated
	}
	actualCosts := calculateActualCosts(r.Context(), application, routeResult, resp, tenantID)

	// Settle reserved funds against the ACTUAL tokens consumed.
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
		log.Printf("gateway: pricing incomplete for dims %v; charging reserved hold %s", actualCosts.MissingPricing, holdAmount)
		finalCost = holdAmount
		walletCharged = holdAmount
	}
	if settleErr := application.Charger.Settle(r.Context(), reserveResult.TransactionID, finalCost); settleErr != nil {
		// Wallet cannot cover a final cost larger than the reserve —
		// commit the reserved amount and RECORD the shortfall in the
		// evidence chain (invariant: errors must never be disguised as
		// success; reconciliation needs to see the undercharge).
		log.Printf("gateway: settle error tx=%s final=%s: %v (falling back to reserved commit)", reserveResult.TransactionID, finalCost, settleErr)
		if commitErr := application.Charger.Commit(r.Context(), reserveResult.TransactionID); commitErr != nil {
			log.Printf("gateway: commit error tx=%s: %v", reserveResult.TransactionID, commitErr)
		}
		walletCharged = holdAmount
		underfunded = true
	}
	// ---- Store response in cache ----
	if cacheSvc != nil && cacheSvc.IsEnabled() && cacheSvc.IsModelAccepted(modelName) {
		if respBodyBytes, jerr := json.Marshal(resp.Body); jerr == nil {
			cached := &cache.CachedResponse{
				StatusCode: resp.StatusCode,
				Body:       string(respBodyBytes),
				Model:      modelName,
			}
			if resp.Usage != nil {
				cached.InputTokens = resp.Usage.InputTokens
				cached.OutputTokens = resp.Usage.OutputTokens
			}
			cacheSvc.Set(r.Context(), cacheKey, cached)
		}
	}

	// Mutate response body before launching the logging goroutine to avoid
	// a data race (logUsage reads resp.Body from a separate goroutine).
	resp.Body["model"] = modelName

	// Record spend against API key limits (best-effort, after settle).
	if !upstreamFailed && actualCosts != nil {
		recordAPIKeySpend(r.Context(), application, apiKeyID, actualCosts.ListCost)
	}

	// Log usage in background with a detached context so it survives
	// the HTTP request lifecycle.
	upstreamModel := stringOrDefault(routeResult.UpstreamModel, modelName)
	go logUsageWithCosts(r, application, "chat", userID, apiKeyID, modelName, upstreamModel, resp, routeResult, actualCosts, walletCharged, underfunded, pricingIncomplete)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if err := json.NewEncoder(w).Encode(resp.Body); err != nil {
		log.Printf("chat: failed to encode response: %v", err)
	}
}

// HandleStreamingChat handles streaming (SSE) chat completion requests with
// budget reserve, usage extraction from stream, and proper billing.
func HandleStreamingChat(w http.ResponseWriter, r *http.Request, application *app.App, modelName string, body map[string]any) {
	cfg := application.Config

	// Resolve routing the same way as non-streaming.
	identity := resolveAuthIdentity(r)
	routeResult, err := application.Router.Route(r.Context(), identity, modelName)
	if err != nil {
		log.Printf("gateway: stream route error: %v", err)
		writeRouteError(w, err)
		return
	}

	baseURL := stringOrDefault(routeResult.Instance.BaseURL, cfg.LiteLLM.BaseURL)
	apiKey := cfg.LiteLLM.MasterKey
	if key, ok := routeResult.Instance.Config["api_key"].(string); ok && key != "" {
		apiKey = key
	}
	upstreamModel := stringOrDefault(routeResult.UpstreamModel, modelName)

	// Resolve tenant ID.
	userID, _ := resolveIdentity(r)
	var tenantID *uuid.UUID
	if identity != nil {
		tenantID = identity.TenantID
	}

	// ---- Budget reserve before upstream call ----
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}

	estimatedUsage := estimateUsageFromBody(body)
	priceResult, err := application.Pricer.Calculate(r.Context(), routeResult.Channel.ModelID, tenantID, estimatedUsage)
	holdAmount := decimal.Zero
	if err != nil {
		log.Printf("gateway: stream pricer estimate error: %v (using minimum hold)", err)
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

	var reserveResult *billing.ReserveResult
	wallet, err := application.Wallets.FindByUser(r.Context(), userID, nil)
	if err != nil {
		log.Printf("gateway: stream wallet lookup error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to verify account")
		return
	}
	if wallet == nil {
		// Fail-closed: every calling account must have a wallet to hold budget.
		writeError(w, http.StatusPaymentRequired, "wallet_missing", "No wallet for this account")
		return
	}
	if !wallet.CanReserve(holdAmount) {
		writeError(w, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance")
		return
	}
	result, err := application.Charger.Reserve(r.Context(), wallet.ID, holdAmount, requestID)
	if err != nil {
		log.Printf("gateway: stream reserve error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Service temporarily unavailable")
		return
	}
	reserveResult = result

	// ---- Build upstream request ----
	body["model"] = upstreamModel
	body["stream"] = true
	// Force the upstream to report a final usage chunk: without
	// stream_options.include_usage most OpenAI-compatible providers omit usage
	// from streaming responses and every stream would fall back to estimates.
	if streamOpts, ok := body["stream_options"].(map[string]any); ok {
		streamOpts["include_usage"] = true
		body["stream_options"] = streamOpts
	} else {
		body["stream_options"] = map[string]any{"include_usage": true}
	}

	reqBytes, err := json.Marshal(body)
	if err != nil {
		releaseIfReserved(r.Context(), application, reserveResult)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to encode request")
		return
	}

	url := strings.TrimSuffix(baseURL, "/v1") + "/v1/chat/completions"
	upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		releaseIfReserved(r.Context(), application, reserveResult)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create upstream request")
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := application.HttpClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	// Track in-flight load for the whole stream duration (best-effort).
	var loadHold *gw.LoadHold
	if application.LoadTracker != nil {
		loadHold, _ = application.LoadTracker.Acquire(r.Context(), routeResult.Instance.ID)
	}
	if loadHold != nil {
		defer loadHold.Release()
	}
	startTime := time.Now()
	resp, err := client.Do(upstreamReq)
	if err != nil {
		log.Printf("gateway: stream upstream error: %v", err)
		releaseIfReserved(r.Context(), application, reserveResult)
		go logStreamFailure(application, userID, resolveStreamAPIKeyID(r), tenantID, modelName, upstreamModel,
			routeResult, body, requestID, "", 0, domain.UsageLogStatusFailed,
			"upstream_error", err.Error(), 0, nil, int(time.Since(startTime).Milliseconds()))
		writeError(w, http.StatusInternalServerError, "upstream_error", "Upstream request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamErrorBody))
		releaseIfReserved(r.Context(), application, reserveResult)
		go logStreamFailure(application, userID, resolveStreamAPIKeyID(r), tenantID, modelName, upstreamModel,
			routeResult, body, requestID, "", 0, domain.UsageLogStatusFailed,
			"upstream_http_error", "upstream returned non-2xx", resp.StatusCode, respBytes, int(time.Since(startTime).Milliseconds()))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBytes)
		return
	}

	// ---- Forward SSE stream while buffering last data chunk ----
	flusher, ok := w.(http.Flusher)
	if !ok {
		releaseIfReserved(r.Context(), application, reserveResult)
		go logStreamFailure(application, userID, resolveStreamAPIKeyID(r), tenantID, modelName, upstreamModel,
			routeResult, body, requestID, "", 0, domain.UsageLogStatusFailed,
			"streaming_not_supported", "streaming not supported by response writer", 0, nil, int(time.Since(startTime).Milliseconds()))
		writeError(w, http.StatusInternalServerError, "internal_error", "Streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	var lastDataLine string
	chunksForwarded := 0
	scanner := bufio.NewScanner(resp.Body)
	// Allow large single chunks (e.g. long tool/function outputs): grow the
	// per-line buffer up to 10 MB instead of failing on the 64 KB default.
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		// Forward SSE data lines.
		if strings.HasPrefix(line, "data:") {
			payload := strings.TrimPrefix(line, "data:")
			payload = strings.TrimSpace(payload)
			// Skip [DONE] marker — we send our own at the end.
			if payload == "[DONE]" {
				continue
			}
			lastDataLine = payload
			chunksForwarded++
			fmt.Fprintf(w, "%s\n\n", line)
		} else {
			fmt.Fprintf(w, "data: %s\n\n", line)
		}
		flusher.Flush()
	}

	// Check for scanner errors (truncated stream / oversized lines) BEFORE
	// sending [DONE]: an incomplete stream must NOT be presented as success
	// (invariant #5 — streaming errors must not masquerade as success).
	if err := scanner.Err(); err != nil {
		log.Printf("gateway: stream scanner error: %v", err)
		// Deliberately omit [DONE] so the client detects the unfinished stream.
		errorCode := "stream_interrupted"
		if r.Context().Err() == context.Canceled || errors.Is(err, context.Canceled) {
			errorCode = "client_disconnected"
		}
		// The client may have disconnected (r.Context() cancelled): release
		// with a detached context so the wallet hold is always compensated.
		relCtx, relCancel := context.WithTimeout(context.Background(), 30*time.Second)
		releaseIfReserved(relCtx, application, reserveResult)
		relCancel()
		go logStreamFailure(application, userID, resolveStreamAPIKeyID(r), tenantID, modelName, upstreamModel,
			routeResult, body, requestID, lastDataLine, chunksForwarded, domain.UsageLogStatusFailed,
			errorCode, err.Error(), resp.StatusCode, nil, int(time.Since(startTime).Milliseconds()))
		return
	}

	// Send [DONE] signal only after a clean EOF.
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	// Parse usage from last buffered SSE data chunk.
	var normUsage *usageparser.NormalizedUsage
	usageSource := usageparser.SourceEstimated
	usageRaw := map[string]any{}

	if lastDataLine != "" {
		var chunk map[string]any
		if json.Unmarshal([]byte(lastDataLine), &chunk) == nil {
			if u, ok := chunk["usage"].(map[string]any); ok {
				usageRaw = u
			}
			nu, err := usageparser.ParseOpenAIUsage(chunk)
			if err == nil && nu.HasUsage() {
				normUsage = nu
				usageSource = usageparser.SourceFinalChunk
			}
		}
	}

	// If no usage in stream, use estimated usage from body.
	if normUsage == nil || !normUsage.HasUsage() {
		normUsage = estimatedUsage
		usageSource = usageparser.SourceEstimated
	}

	// Calculate actual costs from extracted usage.
	var actualCosts *billing.PriceResult
	if normUsage.HasUsage() && application.Pricer != nil {
		result, err := application.Pricer.Calculate(r.Context(), routeResult.Channel.ModelID, tenantID, normUsage)
		if err != nil {
			log.Printf("gateway: stream pricer error: %v", err)
		} else {
			actualCosts = result
		}
	}
	if actualCosts == nil {
		actualCosts = &billing.PriceResult{
			ListCost:      decimal.Zero,
			UpstreamCost:  decimal.Zero,
			ChargeLines:   nil,
			PriceSnapshot: nil,
		}
	}

	// Settle reserved funds against the REAL final cost with a detached
	// context so the settlement succeeds even if the client disconnects
	// mid-stream (r.Context() would be cancelled).
	walletCharged := decimal.Zero
	underfunded := false
	pricingIncomplete := actualCosts != nil && len(actualCosts.MissingPricing) > 0
	if reserveResult != nil {
		commitCtx, commitCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer commitCancel()
		finalCost := decimal.Zero
		if actualCosts != nil {
			finalCost = actualCosts.ListCost
		}
		if pricingIncomplete {
			// Never let a misconfigured price produce a free call: charge the
			// reserved hold and record the evidence for reconciliation.
			log.Printf("gateway: stream pricing incomplete for dims %v; charging reserved hold %s", actualCosts.MissingPricing, holdAmount)
			finalCost = holdAmount
		}
		walletCharged = finalCost
		if settleErr := application.Charger.Settle(commitCtx, reserveResult.TransactionID, finalCost); settleErr != nil {
			// Wallet cannot cover a final cost larger than the reserve —
			// commit the reserved amount and RECORD the shortfall in the
			// evidence chain so reconciliation can see the undercharge.
			log.Printf("gateway: stream settle error tx=%s final=%s: %v (falling back to reserved commit)", reserveResult.TransactionID, finalCost, settleErr)
			if commitErr := application.Charger.Commit(commitCtx, reserveResult.TransactionID); commitErr != nil {
				log.Printf("gateway: stream commit error tx=%s: %v", reserveResult.TransactionID, commitErr)
			}
			walletCharged = holdAmount
			underfunded = true
		}
	}

	// Determine usage source tag.
	domainUsageSource := domain.UsageSourceEstimated
	if usageSource == usageparser.SourceFinalChunk {
		domainUsageSource = domain.UsageSourceFinalChunk
	} else if usageSource == usageparser.SourceUpstream {
		domainUsageSource = domain.UsageSourceUpstream
	}

	// Record spend against API key limits (best-effort, after settle).
	if actualCosts != nil {
		recordAPIKeySpend(context.Background(), application, resolveStreamAPIKeyID(r), actualCosts.ListCost)
	}

	// Build synthetic ExecuteResponse for logging.
	streamResp := &gw.ExecuteResponse{
		StatusCode:    http.StatusOK,
		Body:          map[string]any{"model": modelName, "usage": usageRaw},
		Usage:         normUsage,
		UsageSource:   usageSource,
		DurationMs:    int(time.Since(startTime).Milliseconds()),
		ProviderReqID: requestID,
	}

	// Log usage in background with detached context.
	go logStreamUsage(application, userID, resolveStreamAPIKeyID(r), tenantID, modelName, upstreamModel, streamResp, routeResult, actualCosts, domainUsageSource, domain.UsageLogStatusCompleted, walletCharged, underfunded, pricingIncomplete)
}

// resolveStreamAPIKeyID extracts the API key ID for streaming logging.
func resolveStreamAPIKeyID(r *http.Request) uuid.UUID {
	keyIDStr, _ := r.Context().Value(middleware.CtxAPIKeyID).(string)
	if keyIDStr != "" {
		if id, err := uuid.Parse(keyIDStr); err == nil {
			return id
		}
	}
	return uuid.Nil
}

// releaseIfReserved releases a reserved wallet transaction if it exists and
// returns the quota reservation on upstream failure (compensation).
func releaseIfReserved(ctx context.Context, application *app.App, reserveResult *billing.ReserveResult) {
	if reserveResult != nil {
		if relErr := application.Charger.Release(ctx, reserveResult.TransactionID); relErr != nil {
			log.Printf("gateway: release error tx=%s: %v", reserveResult.TransactionID, relErr)
		}
	}
}

// releaseWalletHold releases a reserved wallet transaction, falling back to a
// detached context when the request context is already cancelled so a client
// disconnect can never leave the hold frozen (invariant #2 compensation).
func releaseWalletHold(ctx context.Context, application *app.App, txID uuid.UUID) {
	relCtx := ctx
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		relCtx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
	}
	if err := application.Charger.Release(relCtx, txID); err != nil {
		log.Printf("gateway: release error tx=%s: %v", txID, err)
	}
}

// estimateUsageFromBody estimates token usage from the request body messages.
// Rough estimation: sum of message content lengths / charsPerToken for input,
// plus estimatedOutputTokens for the expected model output.
func estimateUsageFromBody(body map[string]any) *usageparser.NormalizedUsage {
	nu := &usageparser.NormalizedUsage{}

	messages, ok := body["messages"].([]any)
	if !ok {
		nu.InputTokens = estimatedOutputTokens / 2 // fallback: half of output
		nu.OutputTokens = estimatedOutputTokens
		return nu
	}

	var totalTokens int64
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		content, ok := m["content"].(string)
		if !ok {
			continue
		}
		totalTokens += usageparser.EstimateTextTokens(content)
	}

	nu.InputTokens = totalTokens
	if nu.InputTokens <= 0 {
		nu.InputTokens = 1
	}
	nu.OutputTokens = estimatedOutputTokens
	nu.TotalTokens = nu.InputTokens + nu.OutputTokens

	return nu
}

// calculateActualCosts parses the upstream response usage and runs it through the pricer.
func calculateActualCosts(ctx context.Context, application *app.App, routeResult *gw.RouteResult, resp *gw.ExecuteResponse, tenantID *uuid.UUID) *billing.PriceResult {
	// Parse usage from response body.
	nu, err := usageparser.ParseOpenAIUsage(resp.Body)
	if err != nil || nu == nil || !nu.HasUsage() {
		nu = resp.Usage
	}
	if nu == nil || !nu.HasUsage() {
		// No usable usage data; return zero costs.
		return &billing.PriceResult{
			ListCost:     decimal.Zero,
			UpstreamCost: decimal.Zero,
		}
	}

	// Calculate costs via pricer using the routed model ID.
	if application.Pricer != nil && routeResult != nil && routeResult.Channel != nil {
		result, err := application.Pricer.Calculate(ctx, routeResult.Channel.ModelID, tenantID, nu)
		if err != nil {
			log.Printf("gateway: cost calculation error: %v", err)
			return &billing.PriceResult{ListCost: decimal.Zero, UpstreamCost: decimal.Zero}
		}
		return result
	}

	return &billing.PriceResult{ListCost: decimal.Zero, UpstreamCost: decimal.Zero}
}

// resolveAuthIdentity builds a RequestIdentity from the authorization context
// set by GatewayAuth middleware. Used by the Router for tenant-aware routing.
func resolveAuthIdentity(r *http.Request) *domain.RequestIdentity {
	keyIDStr, _ := r.Context().Value(middleware.CtxAPIKeyID).(string)
	userIDStr, _ := r.Context().Value(middleware.CtxUserID).(string)
	tenantIDStr, _ := r.Context().Value(middleware.CtxTenantID).(string)

	var (
		keyID    uuid.UUID
		userID   uuid.UUID
		tenantID *uuid.UUID
	)
	if keyIDStr != "" {
		if id, err := uuid.Parse(keyIDStr); err == nil {
			keyID = id
		}
	}
	if userIDStr != "" {
		if id, err := uuid.Parse(userIDStr); err == nil {
			userID = id
		}
	}
	if tenantIDStr != "" {
		if id, err := uuid.Parse(tenantIDStr); err == nil {
			tenantID = &id
		}
	}
	return &domain.RequestIdentity{
		APIKeyID:    keyID,
		UserID:      userID,
		TenantID:    tenantID,
		RequestType: "chat",
	}
}

// cacheScope returns a per-tenant, per-user scope string used to isolate
// cached responses between tenants/users (privacy + billing isolation).
func cacheScope(r *http.Request) string {
	identity := resolveAuthIdentity(r)
	scope := ":"
	if identity != nil {
		if identity.TenantID != nil {
			scope = identity.TenantID.String() + ":"
		}
		scope += identity.UserID.String()
	}
	return scope
}

// resolveIdentity extracts the authenticated user and API key from the request context.
func resolveIdentity(r *http.Request) (userID, apiKeyID uuid.UUID) {
	keyIDStr, _ := r.Context().Value(middleware.CtxAPIKeyID).(string)
	userIDStr, _ := r.Context().Value(middleware.CtxUserID).(string)

	if keyIDStr != "" {
		if id, err := uuid.Parse(keyIDStr); err == nil {
			apiKeyID = id
		}
	}
	if userIDStr != "" {
		if id, err := uuid.Parse(userIDStr); err == nil {
			userID = id
		}
	}
	return userID, apiKeyID
}

// stringOrDefault returns s if non-empty, otherwise returns fallback.
func stringOrDefault(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

// upstreamProvider returns the provider name recorded in provider evidence.
// Channel instances created via provider discovery carry their real provider
// (e.g. "deepseek", "bytedance") in config; instances without one are direct
// OpenAI-compatible upstreams ("direct"). A nil route (routing failed before
// any attempt) is recorded as "unknown" so evidence never fabricates a
// provider that was not actually used.
func upstreamProvider(routeResult *gw.RouteResult) string {
	if routeResult == nil || routeResult.Instance == nil {
		return "unknown"
	}
	if p, ok := routeResult.Instance.Config["provider"].(string); ok && p != "" {
		return p
	}
	return "direct"
}

// tenantIDOrDefaultPtr returns tenantID unchanged (nil-safe convenience for
// evidence logging params).
func tenantIDOrDefaultPtr(tenantID *uuid.UUID) *uuid.UUID {
	return tenantID
}

// rejectIncompletePricing fails a request when the pricer could not resolve a
// sell price for a used dimension (invariant: never charge zero silently).
func rejectIncompletePricing(w http.ResponseWriter, priceResult *billing.PriceResult) bool {
	if priceResult == nil || len(priceResult.MissingPricing) == 0 {
		return false
	}
	writeError(w, http.StatusUnprocessableEntity, "pricing_incomplete",
		fmt.Sprintf("No sell price configured for model dimensions: %s", strings.Join(priceResult.MissingPricing, ", ")))
	return true
}

// logUsageWithCosts records the usage log with real costs from the pricer.
// Uses a detached context (30s timeout) independent of the HTTP request lifecycle.
func logUsageWithCosts(r *http.Request, application *app.App, requestType string, userID, apiKeyID uuid.UUID, modelName, upstreamModel string, resp *gw.ExecuteResponse, routeResult *gw.RouteResult, costs *billing.PriceResult, walletCharged decimal.Decimal, underfunded bool, pricingIncomplete bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if costs == nil {
		costs = &billing.PriceResult{ListCost: decimal.Zero, UpstreamCost: decimal.Zero}
	}

	usageSource := domain.UsageSourceUpstream
	if resp.UsageSource == "estimated" {
		usageSource = domain.UsageSourceEstimated
	}

	status := domain.UsageLogStatusCompleted
	errMsg := ""
	if resp.StatusCode >= 400 {
		status = domain.UsageLogStatusFailed
		errMsg = "upstream returned non-2xx"
	}

	usageRaw := map[string]any{}
	if u, ok := resp.Body["usage"].(map[string]any); ok {
		usageRaw = u
	}

	// Use client-supplied X-Request-ID from middleware context, falling back to provider request ID.
	requestID := ""
	if rid, ok := r.Context().Value(middleware.CtxRequestID).(string); ok && rid != "" {
		requestID = rid
	}
	if requestID == "" {
		requestID = resp.ProviderReqID
	}

	// Extract evidence fields from route result when available.
	var channelID, instanceID *uuid.UUID
	if routeResult != nil {
		if routeResult.Channel != nil {
			id := routeResult.Channel.ID
			channelID = &id
		}
		if routeResult.Instance != nil {
			id := routeResult.Instance.ID
			instanceID = &id
		}
	}

	// Compute final cost: pricer ListCost for now.
	finalCost := costs.ListCost

	// Safely get normalized usage JSON.
	normalizedJSON := map[string]any{}
	if resp.Usage != nil {
		normalizedJSON = resp.Usage.ToJSON()
	}

	params := billing.LogUsageParams{
		TenantID:          tenantIDOrDefaultPtr(resolveAuthIdentity(r).TenantID),
		UserID:            userID,
		APIKeyID:          apiKeyID,
		RequestID:         requestID,
		RequestType:       requestType,
		PublicModelCode:   modelName,
		UpstreamModelCode: upstreamModel,
		ProviderRequestID: resp.ProviderReqID,
		ChannelID:         channelID,
		InstanceID:        instanceID,
		UsageSource:       usageSource,
		UsageRaw:          usageRaw,
		UsageNormalized:   normalizedJSON,
		ListCost:          costs.ListCost,
		FinalCost:         finalCost,
		UpstreamCost:      costs.UpstreamCost,
		Currency:          "CNY",
		PriceSnapshot:     costs.PriceSnapshot,
		QuotaDeducted:     0,
		ChargeLines:       costs.ChargeLines,
		Status:            status,
		ErrorCode:         "",
		ErrorMessage:      errMsg,
		WalletCharged:     walletCharged,
		Provider:          upstreamProvider(routeResult),
		ProviderReqID:     resp.ProviderReqID,
		ResponseBody:      resp.Body,
		StatusCode:        resp.StatusCode,
		DurationMs:        resp.DurationMs,
	}
	if underfunded {
		params.ErrorCode = "undercharged"
		params.ErrorMessage = fmt.Sprintf(
			"wallet underfunded: actual=%s charged=%s shortfall=%s",
			finalCost, walletCharged, finalCost.Sub(walletCharged))
	}
	if pricingIncomplete {
		params.ErrorCode = "pricing_incomplete"
		params.ErrorMessage = fmt.Sprintf(
			"pricing incomplete for dimensions: %s; charged reserved hold %s",
			strings.Join(costs.MissingPricing, ","), walletCharged)
	}

	if _, err := application.Logger.Record(ctx, params); err != nil {
		log.Printf("logger record failed: %v", err)
	}
}

// logStreamUsage records usage for streaming requests.
func logStreamUsage(application *app.App, userID, apiKeyID uuid.UUID, tenantID *uuid.UUID, modelName, upstreamModel string, resp *gw.ExecuteResponse, routeResult *gw.RouteResult, costs *billing.PriceResult, usageSource domain.UsageSource, status domain.UsageLogStatus, walletCharged decimal.Decimal, underfunded bool, pricingIncomplete bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if costs == nil {
		costs = &billing.PriceResult{ListCost: decimal.Zero, UpstreamCost: decimal.Zero}
	}

	usageRaw := map[string]any{}
	if u, ok := resp.Body["usage"].(map[string]any); ok {
		usageRaw = u
	}

	var channelID, instanceID *uuid.UUID
	if routeResult != nil {
		if routeResult.Channel != nil {
			id := routeResult.Channel.ID
			channelID = &id
		}
		if routeResult.Instance != nil {
			id := routeResult.Instance.ID
			instanceID = &id
		}
	}

	finalCost := costs.ListCost

	// Safely get normalized usage JSON.
	normalizedJSON := map[string]any{}
	if resp.Usage != nil {
		normalizedJSON = resp.Usage.ToJSON()
	}

	params := billing.LogUsageParams{
		TenantID:          tenantID,
		UserID:            userID,
		APIKeyID:          apiKeyID,
		RequestID:         resp.ProviderReqID,
		RequestType:       "chat",
		PublicModelCode:   modelName,
		UpstreamModelCode: upstreamModel,
		ChannelID:         channelID,
		InstanceID:        instanceID,
		UsageSource:       usageSource,
		UsageRaw:          usageRaw,
		UsageNormalized:   normalizedJSON,
		ListCost:          costs.ListCost,
		FinalCost:         finalCost,
		UpstreamCost:      costs.UpstreamCost,
		Currency:          "CNY",
		PriceSnapshot:     costs.PriceSnapshot,
		QuotaDeducted:     0,
		ChargeLines:       costs.ChargeLines,
		Status:            status,
		ErrorCode:         "",
		ErrorMessage:      "",
		WalletCharged:     walletCharged,
		Provider:          upstreamProvider(routeResult),
		ProviderReqID:     resp.ProviderReqID,
		ResponseBody:      resp.Body,
		StatusCode:        resp.StatusCode,
		DurationMs:        resp.DurationMs,
	}
	if underfunded {
		params.ErrorCode = "undercharged"
		params.ErrorMessage = fmt.Sprintf(
			"wallet underfunded: actual=%s charged=%s shortfall=%s",
			finalCost, walletCharged, finalCost.Sub(walletCharged))
	}
	if pricingIncomplete {
		params.ErrorCode = "pricing_incomplete"
		params.ErrorMessage = fmt.Sprintf(
			"pricing incomplete for dimensions: %s; charged reserved hold %s",
			strings.Join(costs.MissingPricing, ","), walletCharged)
	}

	if _, err := application.Logger.Record(ctx, params); err != nil {
		log.Printf("logger record failed for stream: %v", err)
	}
}

// logStreamFailure records a usage log for failed or interrupted streaming
// requests. It runs with a detached context (30s) so the evidence survives
// client disconnects, and never reports a failure as a successful charge:
// costs and wallet charges are always zero because the reserve was released.
func logStreamFailure(application *app.App, userID, apiKeyID uuid.UUID, tenantID *uuid.UUID, modelName, upstreamModel string, routeResult *gw.RouteResult, body map[string]any, requestID, lastDataLine string, chunksForwarded int, status domain.UsageLogStatus, errorCode, errorMessage string, upstreamStatusCode int, upstreamBody []byte, durationMs int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Chunks were delivered to the client before the interruption: the
	// stream is partial, not a clean failure.
	if status == domain.UsageLogStatusFailed && chunksForwarded > 0 {
		status = domain.UsageLogStatusPartial
	}

	usageSource := domain.UsageSourceEstimated
	usageRaw := map[string]any{}
	if lastDataLine != "" {
		var chunk map[string]any
		if json.Unmarshal([]byte(lastDataLine), &chunk) == nil {
			if u, ok := chunk["usage"].(map[string]any); ok {
				usageRaw = u
			}
			nu, err := usageparser.ParseOpenAIUsage(chunk)
			if err == nil && nu.HasUsage() {
				usageSource = domain.UsageSourceFinalChunk
			}
		}
	}

	responseBody := map[string]any{}
	if len(upstreamBody) > 0 {
		if err := json.Unmarshal(upstreamBody, &responseBody); err != nil {
			responseBody = map[string]any{"raw": string(upstreamBody)}
		}
	}

	var channelID, instanceID *uuid.UUID
	if routeResult != nil {
		if routeResult.Channel != nil {
			id := routeResult.Channel.ID
			channelID = &id
		}
		if routeResult.Instance != nil {
			id := routeResult.Instance.ID
			instanceID = &id
		}
	}

	params := billing.LogUsageParams{
		TenantID:          tenantID,
		UserID:            userID,
		APIKeyID:          apiKeyID,
		RequestID:         requestID,
		RequestType:       "chat",
		PublicModelCode:   modelName,
		UpstreamModelCode: upstreamModel,
		ChannelID:         channelID,
		InstanceID:        instanceID,
		UsageSource:       usageSource,
		UsageRaw:          usageRaw,
		UsageNormalized:   map[string]any{},
		ListCost:          decimal.Zero,
		FinalCost:         decimal.Zero,
		UpstreamCost:      decimal.Zero,
		Currency:          "CNY",
		QuotaDeducted:     0,
		WalletCharged:     decimal.Zero,
		Status:            status,
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		Provider:          upstreamProvider(routeResult),
		ProviderReqID:     requestID,
		RequestBody:       body,
		ResponseBody:      responseBody,
		StatusCode:        upstreamStatusCode,
		DurationMs:        durationMs,
		ProviderErrMsg:    errorMessage,
	}

	if _, err := application.Logger.Record(ctx, params); err != nil {
		log.Printf("logger record failed for stream failure: %v", err)
	}
}

// logNonStreamFailure records a usage log for non-streaming requests that
// failed against every upstream candidate. It runs with a detached context
// (30s) so the evidence survives client disconnects, and always reports zero
// cost: the reserve was released and no tokens were charged. The error body
// is capped at maxUpstreamErrorBody before persistence.
func logNonStreamFailure(application *app.App, requestType string, userID, apiKeyID uuid.UUID, tenantID *uuid.UUID, modelName, upstreamModel string, routeResult *gw.RouteResult, body map[string]any, requestID string, lastErr error, lastResp *gw.ExecuteResponse, attemptCount int, durationMs int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errorCode := "upstream_error"
	errorMessage := ""
	if lastErr != nil {
		errorMessage = lastErr.Error()
	}
	if errors.Is(lastErr, context.Canceled) {
		errorCode = "client_disconnected"
	}
	statusCode := 0
	if lastResp != nil && lastResp.StatusCode >= 400 {
		statusCode = lastResp.StatusCode
		errorCode = "upstream_http_error"
		if errorMessage == "" {
			errorMessage = "upstream returned non-2xx"
		}
		if m, ok := lastResp.Body["error"].(map[string]any); ok {
			if msg, ok := m["message"].(string); ok && msg != "" {
				errorMessage = msg
			}
		} else if s, ok := lastResp.Body["error"].(string); ok && s != "" {
			errorMessage = s
		}
	}
	if attemptCount > 0 && errorMessage != "" {
		errorMessage = fmt.Sprintf("%s (all %d candidates failed)", errorMessage, attemptCount)
	}

	// Prefer the upstream request id (x-request-id from the last failed
	// attempt) so reconciliation can trace back to the provider; fall back to
	// the platform request id when the upstream never answered.
	providerReqID := requestID
	if lastResp != nil && lastResp.ProviderReqID != "" {
		providerReqID = lastResp.ProviderReqID
	}

	responseBody := map[string]any{}
	if lastResp != nil && len(lastResp.Body) > 0 {
		if raw, err := json.Marshal(lastResp.Body); err == nil {
			for {
				if len(raw) > maxUpstreamErrorBody {
					raw = raw[:maxUpstreamErrorBody]
				}
				if err := json.Unmarshal(raw, &responseBody); err != nil {
					responseBody = map[string]any{"raw": string(raw)}
				}
				// Re-encoding the evidence map can add wrapper bytes (e.g. the
				// {"raw": ...} fallback); shrink until it fits the storage cap.
				if b, err := json.Marshal(responseBody); err == nil && len(b) <= maxUpstreamErrorBody {
					break
				}
				if len(raw) <= 64 {
					responseBody = map[string]any{"truncated": true}
					break
				}
				raw = raw[:len(raw)-64]
			}
		}
	}

	var channelID, instanceID *uuid.UUID
	if routeResult != nil {
		if routeResult.Channel != nil {
			id := routeResult.Channel.ID
			channelID = &id
		}
		if routeResult.Instance != nil {
			id := routeResult.Instance.ID
			instanceID = &id
		}
	}

	params := billing.LogUsageParams{
		TenantID:          tenantID,
		UserID:            userID,
		APIKeyID:          apiKeyID,
		RequestID:         requestID,
		RequestType:       requestType,
		PublicModelCode:   modelName,
		UpstreamModelCode: upstreamModel,
		ChannelID:         channelID,
		InstanceID:        instanceID,
		ProviderRequestID: providerReqID,
		UsageSource:       domain.UsageSourceEstimated,
		UsageRaw:          map[string]any{},
		UsageNormalized:   map[string]any{},
		ListCost:          decimal.Zero,
		FinalCost:         decimal.Zero,
		UpstreamCost:      decimal.Zero,
		Currency:          "CNY",
		QuotaDeducted:     0,
		WalletCharged:     decimal.Zero,
		Status:            domain.UsageLogStatusFailed,
		ErrorCode:         errorCode,
		ErrorMessage:      errorMessage,
		Provider:          upstreamProvider(routeResult),
		ProviderReqID:     providerReqID,
		RequestBody:       body,
		ResponseBody:      responseBody,
		StatusCode:        statusCode,
		DurationMs:        durationMs,
		ProviderErrMsg:    errorMessage,
	}

	if _, err := application.Logger.Record(ctx, params); err != nil {
		log.Printf("logger record failed for non-stream failure: %v", err)
	}
}

// logUsageCacheHit logs a zero-cost usage entry for tracking cache-hit metrics.
func logUsageCacheHit(r *http.Request, application *app.App, modelName string, cached *cache.CachedResponse) {
	userID, apiKeyID := resolveIdentity(r)
	identity := resolveAuthIdentity(r)
	var tenantID *uuid.UUID
	if identity != nil {
		tenantID = identity.TenantID
	}

	params := billing.LogUsageParams{
		UserID:            userID,
		APIKeyID:          apiKeyID,
		TenantID:          tenantID,
		PublicModelCode:   modelName,
		UpstreamModelCode: modelName,
		UsageSource:       domain.UsageSourceCached,
		UsageNormalized: map[string]any{
			"input_tokens":  cached.InputTokens,
			"output_tokens": cached.OutputTokens,
			"total_tokens":  cached.InputTokens + cached.OutputTokens,
		},
		FinalCost:      decimal.Zero,
		UpstreamCost:   decimal.Zero,
		RequestID:      r.Header.Get("X-Request-ID"),
		Status:         domain.UsageLogStatusCompleted,
		RequestSummary: `{"cache": "hit"}`,
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := application.Logger.Record(ctx, params); err != nil {
			log.Printf("gateway: cache-hit log error: %v", err)
		}
	}()
}

// sanitizeRequestBody removes client-supplied fields that could override
// upstream routing, authentication, or execution parameters.
func sanitizeRequestBody(body map[string]any) {
	for _, key := range []string{
		"api_key", "api_base", "base_url", "callback_url",
		"headers", "api_version", "user",
	} {
		delete(body, key)
	}
}

// writeRouteError maps routing sentinels to distinct HTTP responses so callers
// can tell "unknown model" from "no access" from "no capacity" instead of a
// single misleading 503.
func writeRouteError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gw.ErrModelNotFound):
		writeError(w, http.StatusNotFound, "model_not_found", "Model not found")
	case errors.Is(err, gw.ErrModelNotActive):
		writeError(w, http.StatusBadRequest, "model_not_active", "Model is not active")
	case errors.Is(err, gw.ErrTenantNotAllowed):
		writeError(w, http.StatusForbidden, "model_not_allowed", "This model is not available to your account")
	case errors.Is(err, gw.ErrNoChannelAvailable):
		writeError(w, http.StatusServiceUnavailable, "no_channel_available", "No provider channel for this model — add API key in Admin Panel")
	default:
		writeError(w, http.StatusInternalServerError, "routing_error", "Internal routing error")
	}
}

func writeError(w http.ResponseWriter, statusCode int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    errType,
		},
	}); err != nil {
		log.Printf("chat: failed to encode error response: %v", err)
	}
}

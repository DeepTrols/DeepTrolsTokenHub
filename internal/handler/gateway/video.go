package gateway

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/provider"
	"github.com/deeptrols/api/internal/service/billing"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HandleVideoGenerations implements POST /v1/videos/generations: creates an
// async generation job (豆包 Seedance / 可灵 style). The upstream call happens
// under a wallet hold; a synchronous result is settled immediately, while an
// upstream task id is recorded as a processing job whose hold is released on
// cancellation or settled on completion (provider-specific polling adapter).
func HandleVideoGenerations(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
			return
		}

		var body map[string]any
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
			return
		}
		modelName, _ := body["model"].(string)
		if modelName == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "model is required")
			return
		}
		n, err := videoCountFromBody(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}

		userID, apiKeyID := resolveIdentity(r)
		candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
		if err != nil {
			log.Printf("gateway: video route error: %v", err)
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

		// Budget reservation precedes the upstream call (invariant #2).
		// Fail-closed hold computation (TH-P05-12 / C-2): an unreliable price
		// rejects the request instead of silently reserving the minimum hold.
		estimatedUsage := &usageparser.NormalizedUsage{VideoUnits: int64(n)}
		holdAmount, holdOK := computeForwardedHold(w, application, r, "videos/generations", primary.Channel.ModelID, tenantID, estimatedUsage)
		if !holdOK {
			return
		}

		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		wallet, werr := application.Wallets.FindByUser(r.Context(), userID, nil)
		if werr != nil {
			log.Printf("gateway: video wallet lookup error: %v", werr)
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

		var (
			reserveResult   *billing.ReserveResult
			resp            *gw.ExecuteResponse
			lastErr         error
			lastRouteResult *gw.RouteResult
		)
		for i := range candidates {
			cand := candidates[i]
			attemptID := requestID
			if i > 0 {
				attemptID = fmt.Sprintf("%s-a%d", requestID, i)
			}
			baseURL := stringOrDefault(cand.Instance.BaseURL, application.Config.LiteLLM.BaseURL)
			apiKey := application.Config.LiteLLM.MasterKey
			if key, ok := cand.Instance.Config["api_key"].(string); ok && key != "" {
				apiKey = key
			}
			upstreamModel := stringOrDefault(cand.UpstreamModel, modelName)

			if !wallet.CanReserve(holdAmount) {
				writeError(w, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance")
				return
			}
			rr, rerr := application.Charger.Reserve(r.Context(), wallet.ID, holdAmount, attemptID)
			if rerr != nil {
				log.Printf("gateway: video reserve error: %v", rerr)
				writeError(w, http.StatusInternalServerError, "internal_error", "Service temporarily unavailable")
				return
			}
			reserveResult = rr

			resp, lastErr = executor.ExecuteEndpoint(r.Context(), baseURL, apiKey, upstreamModel, "videos/generations", body,
				gw.CustomHeadersFromConfig(cand.Instance.Config))
			upstreamFailed := lastErr != nil || (resp != nil && resp.StatusCode >= 400)
			if !upstreamFailed {
				lastRouteResult = &cand
				break
			}
			lastRouteResult = &cand
			releaseWalletHold(r.Context(), application, reserveResult.TransactionID)
			reserveResult = nil
			log.Printf("gateway: video attempt %d failed channel=%q: %v", i, cand.Channel.Name, lastErr)
		}

		if reserveResult == nil {
			msg := "Upstream request failed"
			if lastErr != nil {
				msg = lastErr.Error()
			}
			settleMinuteBucket(r, application, 0)
			writeError(w, http.StatusBadGateway, "upstream_error", msg)
			return
		}

		jobID := uuid.New()

		// Synchronous result (data[].url / b64_json): settle immediately.
		if data, ok := resp.Body["data"].([]any); ok && len(data) > 0 {
			settleVideoEstimate(r, application, resp, lastRouteResult, tenantID, reserveResult, holdAmount, n, apiKeyID, modelName)
			insertAsyncJob(r, application, jobID, userID, apiKeyID, tenantID, modelName, "succeeded", "", "", reserveResult.TransactionID, requestID)
			writeVideoResponse(w, http.StatusOK, jobID, "succeeded", map[string]any{"data": data})
			return
		}

		// Async upstream task id: record a processing job and keep the hold.
		upstreamID := upstreamJobID(resp.Body)
		if upstreamID == "" {
			releaseWalletHold(r.Context(), application, reserveResult.TransactionID)
			settleMinuteBucket(r, application, 0)
			writeError(w, http.StatusBadGateway, "upstream_error", "Unrecognized video generation response")
			return
		}
		insertAsyncJob(r, application, jobID, userID, apiKeyID, tenantID, modelName, "processing", upstreamID, "", reserveResult.TransactionID, requestID)
		writeVideoResponse(w, http.StatusAccepted, jobID, "processing", map[string]any{"task_id": upstreamID})
	}
}

// HandleVideoJobStatus implements GET /v1/videos/generations/{id}.
func HandleVideoJobStatus(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid job ID")
			return
		}
		userID, _ := resolveIdentity(r)

		var (
			status, model, resultURL, upstreamID, errorMsg string
			createdAt, updatedAt                           time.Time
		)
		err = application.Pool.QueryRow(r.Context(),
			`SELECT status, model, COALESCE(result_url,''), COALESCE(upstream_job_id,''), COALESCE(error,''), created_at, updated_at
			 FROM async_jobs WHERE id = $1 AND user_id = $2`, jobID, userID,
		).Scan(&status, &model, &resultURL, &upstreamID, &errorMsg, &createdAt, &updatedAt)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Video job not found")
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{
			"id":         jobID.String(),
			"object":     "video",
			"status":     status,
			"model":      model,
			"result_url": resultURL,
			"task_id":    upstreamID,
			"error":      errorMsg,
			"created_at": createdAt.Format(time.RFC3339),
			"updated_at": updatedAt.Format(time.RFC3339),
		})
	}
}

// HandleVideoJobCancel implements DELETE /v1/videos/generations/{id}.
func HandleVideoJobCancel(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid job ID")
			return
		}
		userID, _ := resolveIdentity(r)

		tx, err := application.Pool.Begin(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to begin transaction")
			return
		}
		defer tx.Rollback(r.Context())

		var status string
		var holdTxID *uuid.UUID
		err = tx.QueryRow(r.Context(),
			`SELECT status, hold_tx_id FROM async_jobs WHERE id = $1 AND user_id = $2 FOR UPDATE`,
			jobID, userID,
		).Scan(&status, &holdTxID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "Video job not found")
			return
		}
		if status != "processing" && status != "queued" {
			writeError(w, http.StatusConflict, "invalid_state", "Job is already "+status)
			return
		}
		if holdTxID != nil {
			releaseWalletHold(r.Context(), application, *holdTxID)
		}
		if _, err := tx.Exec(r.Context(),
			`UPDATE async_jobs SET status = 'cancelled', updated_at = NOW() WHERE id = $1`, jobID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to cancel job")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to commit transaction")
			return
		}
		writeJSONResponse(w, http.StatusOK, map[string]any{"id": jobID.String(), "status": "cancelled"})
	}
}

// videoCountFromBody returns the requested video count (default 1, max 10).
func videoCountFromBody(body map[string]any) (int, error) {
	switch v := body["n"].(type) {
	case nil:
		return 1, nil
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("n must be an integer")
		}
		n := int(v)
		if n < 1 || n > 10 {
			return 0, fmt.Errorf("n must be between 1 and 10")
		}
		return n, nil
	case json.Number:
		if iv, err := v.Int64(); err == nil {
			n := int(iv)
			if n < 1 || n > 10 {
				return 0, fmt.Errorf("n must be between 1 and 10")
			}
			return n, nil
		}
		return 0, fmt.Errorf("n must be an integer")
	default:
		return 0, fmt.Errorf("n must be an integer")
	}
}

// upstreamJobID extracts an upstream task identifier from a video response.
func upstreamJobID(body map[string]any) string {
	if id, ok := body["id"].(string); ok && id != "" {
		return id
	}
	if id, ok := body["task_id"].(string); ok && id != "" {
		return id
	}
	return ""
}

// settleVideoEstimate settles a synchronous video result and logs evidence.
func settleVideoEstimate(
	r *http.Request,
	application *app.App,
	resp *gw.ExecuteResponse,
	routeResult *gw.RouteResult,
	tenantID *uuid.UUID,
	reserveResult *billing.ReserveResult,
	holdAmount decimal.Decimal,
	n int,
	apiKeyID uuid.UUID,
	modelName string,
) decimal.Decimal {
	actualUsage := resp.Usage
	if actualUsage == nil || !actualUsage.HasUsage() {
		actualUsage = &usageparser.NormalizedUsage{VideoUnits: int64(n)}
		resp.Usage = actualUsage
		resp.UsageSource = usageparser.SourceEstimated
	}
	settleMinuteBucket(r, application, actualUsage.TotalTokens)

	actualCosts := calculateActualCosts(r, application, routeResult, resp, tenantID)
	finalCost := decimal.Zero
	if actualCosts != nil {
		finalCost = actualCosts.ListCost
	}
	pricingIncomplete := actualCosts != nil && len(actualCosts.MissingPricing) > 0
	if pricingIncomplete {
		log.Printf("gateway: video pricing incomplete for dims %v; charging reserved hold %s", actualCosts.MissingPricing, holdAmount)
		finalCost = holdAmount
	}
	// A rejected settle falls back through the classifier so an undercharge
	// is always recorded and a replayed request is never debited twice.
	walletCharged, settleEvidence := settleOrFallback(r.Context(), application, "video", modelName, r, reserveResult.TransactionID, finalCost, holdAmount)
	if actualCosts != nil {
		recordAPIKeySpend(r.Context(), application, apiKeyID, actualCosts.ListCost)
	}
	userID, _ := resolveIdentity(r)
	go logUsageWithCosts(r, application, "video", userID, apiKeyID, modelName, stringOrDefault(routeResult.UpstreamModel, modelName),
		resp, routeResult, actualCosts, walletCharged, settleEvidence, pricingIncomplete)
	return finalCost
}

// insertAsyncJob records a video generation job.
func insertAsyncJob(
	r *http.Request,
	application *app.App,
	jobID, userID, apiKeyID uuid.UUID,
	tenantID *uuid.UUID,
	model, status, upstreamID, resultURL string,
	holdTxID uuid.UUID,
	requestID string,
) {
	if application.Pool == nil {
		log.Printf("gateway: skip async job insert (no pool configured)")
		return
	}
	_, err := application.Pool.Exec(r.Context(),
		`INSERT INTO async_jobs (id, user_id, api_key_id, tenant_id, model, status, request_type, upstream_job_id, result_url, hold_tx_id, request_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,'video',$7,$8,$9,$10,NOW(),NOW())`,
		jobID, userID, apiKeyID, tenantID, model, status, upstreamID, resultURL, holdTxID, requestID,
	)
	if err != nil {
		log.Printf("gateway: insert async job: %v", err)
	}
}

// writeVideoResponse writes a standard OpenAI-compatible video response.
func writeVideoResponse(w http.ResponseWriter, status int, jobID uuid.UUID, jobStatus string, extra map[string]any) {
	resp := map[string]any{
		"id":     jobID.String(),
		"object": "video",
		"status": jobStatus,
	}
	for k, v := range extra {
		resp[k] = v
	}
	writeJSONResponse(w, status, resp)
}

// writeJSONResponse encodes a JSON body with the given status code.
func writeJSONResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("gateway: failed to write JSON response: %v", err)
	}
}

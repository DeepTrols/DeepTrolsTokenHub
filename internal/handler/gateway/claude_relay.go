package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/provider"
	"github.com/deeptrols/api/internal/relayconvert"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HandleClaudeMessages implements POST /v1/messages (Anthropic Messages API).
// It converts the Claude request to OpenAI Chat Completions, drives the existing
// reserve → execute → settle billing pipeline against an OpenAI-compatible
// upstream, then converts the response back to Claude format.
func HandleClaudeMessages(application *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var claudeReq relayconvert.ClaudeRequest
		if err := json.NewDecoder(r.Body).Decode(&claudeReq); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
			return
		}
		oaiReq := relayconvert.ClaudeToOpenAIRequest(&claudeReq)
		upstreamBody := structToMap(oaiReq)
		modelName := oaiReq.Model
		if claudeReq.Stream {
			HandleClaudeMessagesStreaming(w, r, application, modelName, upstreamBody)
			return
		}

		cfg := application.Config
		userID, apiKeyID := resolveIdentity(r)
		candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
		if err != nil {
			writeRouteError(w, err)
			return
		}
		candidates = gw.FilterByGroup(candidates, apiKeyGroup(r))
		if len(candidates) == 0 {
			writeRouteError(w, gw.ErrNoChannelAvailable)
			return
		}
		primary := candidates[0]
		var tenantID *uuid.UUID
		if identity := resolveAuthIdentity(r); identity != nil {
			tenantID = identity.TenantID
		}

		// Reserve BEFORE upstream (invariant #2): maximum-charge hold
		// (TH-P05-01) on the converted chat body (carries max_tokens),
		// fail-closed on unreliable pricing.
		hold, _, ok := computeMaxChargeHold(w, application, r, primary.Channel.ModelID, tenantID, upstreamBody)
		if !ok {
			return
		}
		wallet, err := application.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil || wallet == nil {
			writeError(w, http.StatusPaymentRequired, "wallet_missing", "No wallet for this account")
			return
		}
		if !wallet.CanReserve(hold) {
			writeError(w, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance")
			return
		}
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		rr, rerr := application.Charger.Reserve(r.Context(), wallet.ID, hold, requestID)
		if rerr != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Service temporarily unavailable")
			return
		}

		executor := application.Executor
		if executor == nil {
			executor = provider.NewExecutorForConfig(primary.Instance.Config)
		}
		baseURL := stringOrDefault(primary.Instance.BaseURL, cfg.LiteLLM.BaseURL)
		apiKey := cfg.LiteLLM.MasterKey
		if k, ok := primary.Instance.Config["api_key"].(string); ok && k != "" {
			apiKey = k
		}
		upstreamModel := stringOrDefault(primary.UpstreamModel, modelName)
		resp, err := executor.ExecuteEndpoint(r.Context(), baseURL, apiKey, upstreamModel, "chat/completions", upstreamBody,
			gw.CustomHeadersFromConfig(primary.Instance.Config))
		if err != nil || resp == nil || resp.StatusCode >= 400 {
			_ = application.Charger.Release(r.Context(), rr.TransactionID)
			msg := "Upstream request failed"
			if err != nil {
				msg = err.Error()
			}
			writeError(w, http.StatusBadGateway, "upstream_error", msg)
			return
		}

		actualUsage := resp.Usage
		if actualUsage == nil || !actualUsage.HasUsage() {
			actualUsage = &usageparser.NormalizedUsage{}
			resp.Usage = actualUsage
		}
		settleMinuteBucket(r, application, actualUsage.TotalTokens)
		actualCosts := calculateActualCosts(r, application, &primary, resp, tenantID)
		finalCost := decimal.Zero
		pricingIncomplete := false
		if actualCosts != nil {
			finalCost = actualCosts.ListCost
			pricingIncomplete = len(actualCosts.MissingPricing) > 0
		}
		// Subscription free-token allowance: a request inside the remaining
		// quota settles at zero (evidence still records the real usage).
		quotaCovered := false
		if adj, covered := applySubscriptionAllowance(r.Context(), application, userID.String(), actualUsage.TotalTokens, finalCost); covered {
			finalCost = adj
			quotaCovered = true
		}
		if pricingIncomplete && !quotaCovered {
			finalCost = hold
		}
		// A rejected settle falls back through the classifier so an
		// undercharge is always recorded and a replayed request is never
		// debited twice (TH-P05-02).
		walletCharged, settleEvidence := settleOrFallback(r.Context(), application, "messages", modelName, r, rr.TransactionID, finalCost, hold)
		if actualCosts != nil {
			recordAPIKeySpend(r.Context(), application, apiKeyID, actualCosts.ListCost)
		}
		go logUsageWithCosts(r, application, "chat", userID, apiKeyID, modelName, upstreamModel, resp, &primary, actualCosts, walletCharged, settleEvidence, pricingIncomplete)

		oaiResp := &relayconvert.OpenAIChatResponse{Model: upstreamModel}
		parseOpenAIResponseInto(resp.Body, oaiResp)
		writeJSONResponse(w, http.StatusOK, relayconvert.OpenAIToClaudeResponse(oaiResp))
	}
}

// claudeMessagesStreamOutput converts upstream OpenAI SSE chunks into
// Anthropic Messages SSE events (relaykit oai_chat → claude_messages parity).
type claudeMessagesStreamOutput struct {
	state *relayconvert.ClaudeStreamState
}

func (o *claudeMessagesStreamOutput) writeData(w io.Writer, payload string) error {
	var chunk map[string]any
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		// Non-JSON upstream payload: forward verbatim rather than dropping it.
		_, werr := fmt.Fprintf(w, "data: %s\n\n", payload)
		return werr
	}
	for _, evt := range relayconvert.OpenAIStreamChunkToClaudeEvents(chunk, o.state) {
		b, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		typ, _ := evt["type"].(string)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, b); err != nil {
			return err
		}
	}
	return nil
}

func (o *claudeMessagesStreamOutput) writeDone(w io.Writer) error {
	if o.state.Done {
		// The converter already emitted the full terminal sequence
		// (content_block_stop + message_delta + message_stop).
		return nil
	}
	// Clean EOF without a terminal chunk: close open blocks and finish the
	// message so the Anthropic client sees a well-formed end-of-stream.
	for _, evt := range o.state.ForceFinishEvents() {
		b, err := json.Marshal(evt)
		if err != nil {
			return err
		}
		typ, _ := evt["type"].(string)
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, b); err != nil {
			return err
		}
	}
	return nil
}

// claudeStreamRoute resolves the primary route respecting the API key group,
// matching the non-streaming /v1/messages path.
func claudeStreamRoute(application *app.App, r *http.Request, modelName string) (*gw.RouteResult, error) {
	candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
	if err != nil {
		return nil, err
	}
	candidates = gw.FilterByGroup(candidates, apiKeyGroup(r))
	if len(candidates) == 0 {
		return nil, gw.ErrNoChannelAvailable
	}
	return &candidates[0], nil
}

// HandleClaudeMessagesStreaming implements streaming POST /v1/messages: the
// Claude request (already converted to an OpenAI chat body) streams through the
// OpenAI-compatible upstream and every chunk is converted back to Anthropic
// Messages SSE. Billing follows the shared stream pipeline (final-chunk usage).
func HandleClaudeMessagesStreaming(w http.ResponseWriter, r *http.Request, application *app.App, modelName string, body map[string]any) {
	body["stream"] = true
	handleStreaming(w, r, application, modelName, body, streamConfig{
		out:          &claudeMessagesStreamOutput{state: &relayconvert.ClaudeStreamState{}},
		resolveRoute: claudeStreamRoute,
	})
}

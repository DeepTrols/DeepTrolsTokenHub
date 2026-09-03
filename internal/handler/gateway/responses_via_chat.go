package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/metrics"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/provider"
	"github.com/deeptrols/api/internal/relayconvert"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// responsesViaChat reports whether the routed channel is chat-completions-only
// (instance config "responses_via_chat": true), i.e. the /v1/responses request
// must be converted to a chat/completions call (new-api responses_via_chat).
func responsesViaChat(application *app.App, r *http.Request, modelName string) bool {
	candidates, err := application.Router.RouteCandidates(r.Context(), resolveAuthIdentity(r), modelName, 3)
	if err != nil {
		return false
	}
	candidates = gw.FilterByGroup(candidates, apiKeyGroup(r))
	if len(candidates) == 0 {
		return false
	}
	return configBool(candidates[0].Instance.Config, "responses_via_chat")
}

// configBool tolerates JSON booleans and JSON-string booleans in instance
// config (the admin API persists config values as strings).
func configBool(config map[string]any, key string) bool {
	switch v := config[key].(type) {
	case bool:
		return v
	case string:
		b, err := strconv.ParseBool(v)
		return err == nil && b
	}
	return false
}

// responsesBodyToChatBody converts a Responses request body to an OpenAI chat
// request body (Responses → chat).
func responsesBodyToChatBody(body map[string]any) (map[string]any, error) {
	// OpenAI Responses accepts `input` as a plain string (convenience form);
	// normalize it to a user message item before conversion. The item MUST
	// carry type=message, otherwise ResponsesRequestToOpenAIChatRequest
	// silently drops it and the upstream chat call loses the prompt (and the
	// hold calculation loses the prompt estimate).
	if s, ok := body["input"].(string); ok {
		body["input"] = []any{map[string]any{
			"type":    "message",
			"role":    "user",
			"content": []any{map[string]any{"type": "input_text", "text": s}},
		}}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var rr relayconvert.ResponsesRequest
	if err := json.Unmarshal(b, &rr); err != nil {
		return nil, err
	}
	return structToMap(relayconvert.ResponsesRequestToOpenAIChatRequest(&rr)), nil
}

// responsesStreamOutput converts upstream OpenAI chat.completion.chunk SSE to
// OpenAI Responses SSE events.
type responsesStreamOutput struct {
	state *relayconvert.ResponsesStreamState
}

func (o *responsesStreamOutput) writeData(w io.Writer, payload string) error {
	var chunk map[string]any
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		_, werr := fmt.Fprintf(w, "data: %s\n\n", payload)
		return werr
	}
	for _, evt := range relayconvert.OpenAIStreamChunkToResponsesEvents(chunk, o.state) {
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

func (o *responsesStreamOutput) writeDone(w io.Writer) error {
	if o.state.Done {
		return nil
	}
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

// HandleResponsesViaChat implements POST /v1/responses against a
// chat-completions-only upstream: the Responses request is converted to a chat
// request, the upstream chat response is converted back (streaming or not) and
// billing follows the standard reserve → execute → settle evidence pipeline.
func HandleResponsesViaChat(w http.ResponseWriter, r *http.Request, application *app.App, modelName string, body map[string]any) {
	chatBody, err := responsesBodyToChatBody(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid responses request body")
		return
	}
	stream, _ := body["stream"].(bool)
	if stream {
		chatBody["stream"] = true
		handleStreaming(w, r, application, modelName, chatBody, streamConfig{
			out:          &responsesStreamOutput{state: &relayconvert.ResponsesStreamState{}},
			resolveRoute: claudeStreamRoute,
		})
		return
	}
	handleResponsesViaChatNonStream(w, r, application, modelName, chatBody)
}

// handleResponsesViaChatNonStream runs the non-streaming chat pipeline and
// converts the chat completion back to a Responses response.
func handleResponsesViaChatNonStream(w http.ResponseWriter, r *http.Request, application *app.App, modelName string, chatBody map[string]any) {
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

	// Reserve BEFORE upstream (invariant #2): maximum-charge hold (TH-P05-01),
	// fail-closed on unreliable pricing.
	hold, _, ok := computeMaxChargeHold(w, application, r, primary.Channel.ModelID, tenantID, chatBody)
	if !ok {
		return
	}
	wallet, err := application.Wallets.FindByUser(r.Context(), userID, nil)
	if err != nil || wallet == nil {
		metrics.IncProviderBlocked("responses", metrics.ReasonWalletMissing)
		writeError(w, http.StatusPaymentRequired, "wallet_missing", "No wallet for this account")
		return
	}
	if !wallet.CanReserve(hold) {
		metrics.IncProviderBlocked("responses", metrics.ReasonInsufficientBalance)
		writeError(w, http.StatusPaymentRequired, "insufficient_balance", "Insufficient balance")
		return
	}
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = uuid.New().String()
	}
	rr, rerr := application.Charger.Reserve(r.Context(), wallet.ID, hold, requestID)
	if rerr != nil {
		metrics.IncProviderBlocked("responses", metrics.ReasonReserveFailed)
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
	resp, err := executor.ExecuteEndpoint(r.Context(), baseURL, apiKey, upstreamModel, "chat/completions", chatBody,
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
	// Subscription free-token allowance: a request inside the remaining quota
	// settles at zero (evidence still records the real usage).
	quotaCovered := false
	if adj, covered := applySubscriptionAllowance(r.Context(), application, userID.String(), actualUsage.TotalTokens, finalCost); covered {
		finalCost = adj
		quotaCovered = true
	}
	if pricingIncomplete && !quotaCovered {
		finalCost = hold
	}
	// A rejected settle falls back through the classifier so an undercharge
	// is always recorded and a replayed request is never debited twice
	// (TH-P05-02).
	walletCharged, settleEvidence := settleOrFallback(r.Context(), application, "responses", modelName, r, rr.TransactionID, finalCost, hold)
	if actualCosts != nil {
		recordAPIKeySpend(r.Context(), application, apiKeyID, actualCosts.ListCost)
	}
	go logUsageWithCosts(r, application, "chat", userID, apiKeyID, modelName, upstreamModel, resp, &primary, actualCosts, walletCharged, settleEvidence, pricingIncomplete)

	oaiResp := &relayconvert.OpenAIChatResponse{Model: upstreamModel}
	parseOpenAIResponseInto(resp.Body, oaiResp)
	writeJSONResponse(w, http.StatusOK, relayconvert.OpenAIChatResponseToResponses(oaiResp))
}

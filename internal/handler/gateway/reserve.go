package gateway

import (
	"log"
	"net/http"
	"strconv"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Maximum-charge budget hold (TH-P05-01 / B5).
//
// Before any upstream call the gateway reserves the MAXIMUM possible charge
// of the request instead of a rough minimum:
//
//	holdUsage.InputTokens  = prompt estimate (same as estimateUsageFromBody)
//	holdUsage.OutputTokens = declared max_completion_tokens / max_tokens,
//	                         capped at maxChargeOutputCap; when absent or
//	                         malformed the documented fallbackOutputTokens.
//	holdAmount             = pricer list cost of holdUsage, floored at
//	                         minHoldAmount.
//
// Fail-closed: when the price cannot be computed reliably (pricer error or a
// used dimension without a sell price) the request is rejected BEFORE any
// reserve transaction and BEFORE any upstream call — the previous silent
// fallback to minHoldAmount was the B5 under-reserve hole.

const (
	// fallbackOutputTokens is the documented output bound used when a request
	// declares no usable max_tokens / max_completion_tokens. It keeps the
	// previous safe estimate; the resulting hold is additionally floored at
	// minHoldAmount so it can never fall below the previous minimum hold.
	fallbackOutputTokens = int64(estimatedOutputTokens)

	// maxChargeOutputCap bounds the declared output tokens used for the hold
	// so an absurd max_tokens declaration cannot force an unbounded reserve.
	// 131072 covers the largest current output limits of hosted models.
	maxChargeOutputCap = int64(131072)
)

// Hold calculation modes, exposed for tests and observability.
const (
	// holdModeDeclaredMax: hold output bound comes from the request's
	// declared max_completion_tokens / max_tokens.
	holdModeDeclaredMax = "declared_max"
	// holdModeFallbackCap: no usable declaration; the documented fallback
	// output bound applies.
	holdModeFallbackCap = "fallback_cap"
)

// maxHoldOutputTokens returns the output token bound used for the
// maximum-charge hold of a chat-shaped request body plus the mode that
// produced it. max_completion_tokens wins over max_tokens (OpenAI semantics:
// it is the non-deprecated field and covers reasoning tokens as well).
// Non-positive, non-numeric or otherwise malformed declarations fall back to
// the documented cap instead of silently under-reserving.
func maxHoldOutputTokens(body map[string]any) (int64, string) {
	for _, key := range []string{"max_completion_tokens", "max_tokens"} {
		if v, ok := declaredOutputTokens(body[key]); ok {
			if v > maxChargeOutputCap {
				v = maxChargeOutputCap
			}
			return v, holdModeDeclaredMax
		}
	}
	return fallbackOutputTokens, holdModeFallbackCap
}

// declaredOutputTokens coerces a raw request-body value to a positive declared
// output token count. JSON numbers decode as float64; integer-valued strings
// are accepted as a client courtesy. Everything else (absent, nil, zero,
// negative, non-numeric) counts as "not declared".
func declaredOutputTokens(raw any) (int64, bool) {
	switch v := raw.(type) {
	case float64:
		if v < 1 {
			return 0, false
		}
		return int64(v), true
	case int:
		if v < 1 {
			return 0, false
		}
		return int64(v), true
	case int64:
		if v < 1 {
			return 0, false
		}
		return v, true
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 {
			return 0, false
		}
		return n, true
	}
	return 0, false
}

// holdUsageFromChatBody builds the worst-case usage for hold calculation from
// a chat-shaped request body: the same prompt estimate used today plus the
// declared (or fallback) output bound. estimateUsageFromBody itself stays
// unchanged so non-billing consumers (API key boundary checks) keep their
// behavior.
func holdUsageFromChatBody(body map[string]any) (*usageparser.NormalizedUsage, string) {
	base := estimateUsageFromBody(body)
	out, mode := maxHoldOutputTokens(body)
	hold := *base
	hold.OutputTokens = out
	hold.TotalTokens = hold.InputTokens + hold.OutputTokens
	return &hold, mode
}

// computeMaxChargeHold prices the worst-case (maximum-charge) usage of a
// chat-shaped body and returns the amount to reserve before the upstream
// call, plus the hold calculation mode. It fails closed: on a pricer error or
// any missing price dimension it writes the error response and returns
// ok=false — callers must return without reserving or calling upstream. On
// success the amount is never below minHoldAmount.
func computeMaxChargeHold(w http.ResponseWriter, application *app.App, r *http.Request, modelID uuid.UUID, tenantID *uuid.UUID, body map[string]any) (decimal.Decimal, string, bool) {
	holdUsage, mode := holdUsageFromChatBody(body)
	priceResult, err := priceWithAdjustments(application, r, modelID, tenantID, holdUsage)
	if err != nil {
		// Fail-closed: never reserve (and never call upstream) on an
		// unreliable price. This replaces the silent minHoldAmount fallback.
		log.Printf("gateway: pricer error during hold calculation: %v (fail-closed, no reserve)", err)
		writeError(w, http.StatusUnprocessableEntity, "pricing_incomplete", "Unable to price request reliably")
		return decimal.Zero, mode, false
	}
	if rejectIncompletePricing(w, priceResult) {
		return decimal.Zero, mode, false
	}
	// The hold is the priced maximum charge, but never below the previous
	// minimum hold (AC-03): a reserve smaller than minHoldAmount protects
	// nothing. Realistic pricing keeps the formula dominant; the floor is a
	// safety net for very cheap models.
	hold := priceResult.ListCost
	if minHold := decimal.RequireFromString(minHoldAmount); hold.LessThan(minHold) {
		hold = minHold
	}
	// Observability without request body text: calculation mode, pricing
	// completeness and a low-cardinality reserve amount bucket.
	log.Printf("gateway: hold_calc mode=%s pricing_complete=true reserve_bucket=%s",
		mode, reserveAmountBucket(hold))
	return hold, mode, true
}

// reserveAmountBucket maps a hold amount to a low-cardinality label for
// observability (never logs the exact amount together with request content).
func reserveAmountBucket(amount decimal.Decimal) string {
	switch {
	case amount.LessThan(decimal.RequireFromString("0.001")):
		return "lt_0.001"
	case amount.LessThan(decimal.RequireFromString("0.01")):
		return "0.001_0.01"
	case amount.LessThan(decimal.RequireFromString("0.1")):
		return "0.01_0.1"
	case amount.LessThan(decimal.NewFromInt(1)):
		return "0.1_1"
	default:
		return "gte_1"
	}
}

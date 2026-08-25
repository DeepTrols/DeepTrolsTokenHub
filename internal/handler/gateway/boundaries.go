package gateway

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/minutebucket"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// boundaryError is a typed API key boundary violation that maps to an HTTP error.
type boundaryError struct {
	status  int
	errType string
	message string
}

func (e *boundaryError) Error() string { return e.message }

// enforceAPIKeyBoundaries applies the API key governance boundaries that were
// previously stored but never enforced on the execution path:
//  1. model allowlist        (AllowedModels)
//  2. source IP whitelist    (SourceWhitelist)
//  3. cumulative/weekly/monthly spend limits (OverLimitAction: block | warn)
//
// Called before routing / billing / caching so every request (including cache
// hits) is subject to the same policy.
func enforceAPIKeyBoundaries(w http.ResponseWriter, r *http.Request, application *app.App, modelName string, estimatedTokens int64) error {
	if application.APIKeys == nil {
		return nil // policy repository unavailable (should not happen in prod)
	}

	keyIDStr, _ := r.Context().Value(middleware.CtxAPIKeyID).(string)
	if keyIDStr == "" {
		return nil // no key identity (should not happen under GatewayAuth)
	}
	keyID, err := uuid.Parse(keyIDStr)
	if err != nil {
		return nil
	}

	key, err := application.APIKeys.FindByID(r.Context(), keyID)
	if err != nil || key == nil {
		return fmt.Errorf("apikey lookup for boundary check: %w", err)
	}

	// 0. RPM/TPM minute bucket (Phase 1). Degrades fail-open with a log when
	// the bucket store errors; over-limit is a hard 429 with rate-limit headers.
	if application.MinuteBuckets != nil && (key.RateLimitRPM > 0 || key.RateLimitTPM > 0) {
		result, berr := application.MinuteBuckets.Reserve(r.Context(), key.ID.String(),
			estimatedTokens, key.RateLimitRPM, key.RateLimitTPM, time.Now().UTC())
		if berr != nil {
			log.Printf("gateway: minute bucket degraded for key %s: %v", keyID, berr)
		} else {
			writeRateLimitHeaders(w, key.RateLimitRPM, key.RateLimitTPM, result, time.Now().UTC())
			if !result.Allowed {
				return &boundaryError{http.StatusTooManyRequests, "rate_limit_exceeded",
					"API key rate limit exceeded (RPM/TPM)"}
			}
		}
	}

	// 1. Model allowlist.
	if len(key.AllowedModels) > 0 {
		allowed := false
		for _, m := range key.AllowedModels {
			if m == modelName {
				allowed = true
				break
			}
		}
		if !allowed {
			return &boundaryError{http.StatusForbidden, "model_not_allowed", "Model not allowed for this API key"}
		}
	}

	// 2. Source IP whitelist.
	if len(key.SourceWhitelist) > 0 {
		ip := clientIP(r)
		ok := false
		for _, allowed := range key.SourceWhitelist {
			if allowed == ip {
				ok = true
				break
			}
		}
		if !ok {
			return &boundaryError{http.StatusForbidden, "ip_not_allowed", "Source IP is not allowed for this API key"}
		}
	}

	// 3. Spend limits (cumulative / weekly / monthly).
	for _, p := range []struct {
		typ   string
		limit decimal.Decimal
	}{
		{"cumulative", key.CumulativeLimit},
		{"weekly", key.WeeklyLimit},
		{"monthly", key.MonthlyLimit},
	} {
		if !p.limit.IsPositive() {
			continue // no limit configured
		}
		spend, err := application.APIKeys.GetSpend(r.Context(), keyID, p.typ)
		if err != nil {
			log.Printf("gateway: spend lookup %s key=%s: %v", p.typ, keyID, err)
			continue
		}
		if spend.TotalCost.GreaterThanOrEqual(p.limit) {
			if key.OverLimitAction == domain.OverLimitBlock {
				return &boundaryError{http.StatusForbidden, "limit_exceeded", fmt.Sprintf("%s spend limit reached for this API key", p.typ)}
			}
			// warn: allow the request but surface the breach.
			log.Printf("gateway: api key %s over %s limit (action=warn) limit=%s spent=%s", keyID, p.typ, p.limit, spend.TotalCost)
		}
	}

	return nil
}

// writeRateLimitHeaders mirrors TokenHub's X-RateLimit-* header contract so
// clients can back off deterministically.
func writeRateLimitHeaders(w http.ResponseWriter, rpmLimit int, tpmLimit int64, result minutebucket.Result, now time.Time) {
	reset := int64(60 - now.UTC().Second())
	if reset < 1 {
		reset = 1
	}
	if rpmLimit > 0 {
		w.Header().Set("X-RateLimit-Limit-Requests", fmt.Sprintf("%d", rpmLimit))
		w.Header().Set("X-RateLimit-Remaining-Requests", fmt.Sprintf("%d", max64(int64(rpmLimit)-result.Requests, 0)))
	}
	if tpmLimit > 0 {
		w.Header().Set("X-RateLimit-Limit-Tokens", fmt.Sprintf("%d", tpmLimit))
		w.Header().Set("X-RateLimit-Remaining-Tokens", fmt.Sprintf("%d", max64(tpmLimit-result.Tokens, 0)))
	}
	w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", reset))
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// recordAPIKeySpend accumulates the final cost against the key's
// cumulative/weekly/monthly spend buckets. Best-effort: failures are logged,
// not propagated (the charge itself is already settled).
func recordAPIKeySpend(ctx context.Context, application *app.App, keyID uuid.UUID, cost decimal.Decimal) {
	if keyID == uuid.Nil || cost.IsZero() || application.APIKeys == nil {
		return
	}

	now := time.Now().UTC()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	epoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)

	spends := []*domain.APIKeySpend{
		{APIKeyID: keyID, PeriodType: "cumulative", PeriodStart: &epoch, TotalCost: cost},
		{APIKeyID: keyID, PeriodType: "weekly", PeriodStart: &weekStart, TotalCost: cost},
		{APIKeyID: keyID, PeriodType: "monthly", PeriodStart: &monthStart, TotalCost: cost},
	}
	for _, s := range spends {
		if err := application.APIKeys.UpdateSpend(ctx, s); err != nil {
			log.Printf("gateway: record spend %s key=%s: %v", s.PeriodType, keyID, err)
		}
	}
}

// clientIP extracts the client IP from RemoteAddr (strips port). Trusting
// RemoteAddr over X-Forwarded-For avoids spoofing when the API is exposed
// directly; behind a trusted proxy, configure it to set RemoteAddr properly.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

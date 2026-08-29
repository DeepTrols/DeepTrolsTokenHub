package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/service/billing"
	"github.com/deeptrols/api/internal/service/setting"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// gatewayZone is the billing timezone for the volume-discount monthly counter.
var gatewayZone = time.FixedZone("Asia/Shanghai", 8*3600)

// monthlyUsageCacheTTL bounds staleness of the volume-discount monthly token
// counter; a tier boundary may take up to this long to apply after crossing.
const monthlyUsageCacheTTL = 5 * time.Minute

// groupRatio resolves the pricing multiplier for the request's API-key group
// from the user_groups setting (new-api group_ratio parity). Unset, unknown or
// malformed entries fall back to ratio 1 — never to a negative/zero price.
func groupRatio(application *app.App, r *http.Request) decimal.Decimal {
	group := apiKeyGroup(r)
	if group == "" || application == nil || application.Settings == nil {
		return decimal.NewFromInt(1)
	}
	all, err := application.Settings.All(r.Context())
	if err != nil {
		return decimal.NewFromInt(1)
	}
	raw, ok := all[setting.KeyUserGroups]
	if !ok {
		return decimal.NewFromInt(1)
	}
	var groups []struct {
		Name  string `json:"name"`
		Ratio string `json:"ratio"`
	}
	if json.Unmarshal(raw, &groups) != nil {
		return decimal.NewFromInt(1)
	}
	for _, g := range groups {
		if g.Name != group {
			continue
		}
		ratio, err := decimal.NewFromString(g.Ratio)
		if err != nil || !ratio.IsPositive() {
			return decimal.NewFromInt(1)
		}
		return ratio
	}
	return decimal.NewFromInt(1)
}

// volumeRatio resolves the volume-discount multiplier for the request: the
// best tier whose min_tokens is covered by the user's cumulative completed
// token usage in the current GMT+8 month. Unset/unknown/malformed → 1.
func volumeRatio(application *app.App, r *http.Request) decimal.Decimal {
	if application == nil || application.Settings == nil || application.Pool == nil {
		return decimal.NewFromInt(1)
	}
	// The bootstrap admin account legitimately owns the nil UUID, so identity
	// presence is checked via the context value, not by comparing to uuid.Nil.
	userIDStr, ok := r.Context().Value(middleware.CtxUserID).(string)
	if !ok || userIDStr == "" {
		return decimal.NewFromInt(1)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return decimal.NewFromInt(1)
	}
	all, err := application.Settings.All(r.Context())
	if err != nil {
		return decimal.NewFromInt(1)
	}
	raw, ok := all[setting.KeyDiscountTiers]
	if !ok {
		return decimal.NewFromInt(1)
	}
	var tiers []struct {
		MinTokens int64  `json:"min_tokens"`
		Ratio     string `json:"ratio"`
	}
	if json.Unmarshal(raw, &tiers) != nil || len(tiers) == 0 {
		return decimal.NewFromInt(1)
	}

	now := time.Now().In(gatewayZone)
	monthKey := now.Format("200601")
	cacheKey := "deeptrols:usage:month:" + userID.String() + ":" + monthKey
	var total int64
	cached := false
	if application.Redis != nil {
		if v, err := application.Redis.Get(r.Context(), cacheKey).Int64(); err == nil {
			total = v
			cached = true
		}
	}
	if !cached {
		monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, gatewayZone)
		if err := application.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM((usage_normalized->>'total_tokens')::bigint), 0) FROM usage_logs
			 WHERE user_id = $1 AND status = 'completed' AND created_at >= $2`,
			userID, monthStart).Scan(&total); err != nil {
			return decimal.NewFromInt(1)
		}
		if application.Redis != nil {
			application.Redis.Set(r.Context(), cacheKey, total, monthlyUsageCacheTTL)
		}
	}

	best := decimal.NewFromInt(1)
	for _, tier := range tiers {
		if tier.MinTokens > total {
			continue
		}
		if ratio, err := decimal.NewFromString(tier.Ratio); err == nil && ratio.IsPositive() {
			best = ratio
		}
	}
	return best
}

// priceWithAdjustments prices usage applying the combined multiplier of the
// API-key group ratio and the volume-discount tier, so the gateway keeps a
// single billing entry point.
func priceWithAdjustments(application *app.App, r *http.Request, modelID uuid.UUID, tenantID *uuid.UUID, usage *usageparser.NormalizedUsage) (*billing.PriceResult, error) {
	ratio := groupRatio(application, r).Mul(volumeRatio(application, r))
	return application.Pricer.CalculateWithRatio(r.Context(), modelID, tenantID, usage, ratio)
}

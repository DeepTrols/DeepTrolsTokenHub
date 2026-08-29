package gateway

import (
	"context"

	"github.com/deeptrols/api/internal/app"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// applySubscriptionAllowance waives the full request cost when the user's
// active subscription has enough free-token quota remaining. It returns the
// adjusted final cost and whether the quota covered the request. Any failure
// (no quota plan, exhausted allowance, DB error) bills normally — a quota
// hiccup must never produce a free call.
func applySubscriptionAllowance(ctx context.Context, a *app.App, userID string, totalTokens int64, finalCost decimal.Decimal) (decimal.Decimal, bool) {
	if a == nil || a.Subscriptions == nil || userID == "" || totalTokens <= 0 || finalCost.IsZero() {
		return finalCost, false
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return finalCost, false
	}
	remaining, ok, err := a.Subscriptions.RemainingQuota(ctx, uid)
	if err != nil || !ok || remaining < totalTokens {
		return finalCost, false
	}
	consumed, err := a.Subscriptions.ConsumeQuota(ctx, uid, totalTokens)
	if err != nil || consumed == 0 {
		return finalCost, false
	}
	return decimal.Zero, true
}

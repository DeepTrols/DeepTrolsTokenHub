package console

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/wallet"
	subscriptionsvc "github.com/deeptrols/api/internal/service/subscription"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// flexInt accepts a JSON number or a numeric string (e.g. "30"), so admin
// forms that keep inputs as strings do not fail strict integer decoding.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	n, err := parseFlexInt(b)
	if err != nil {
		return err
	}
	*f = flexInt(n)
	return nil
}

func (f *flexInt) Int() int {
	if f == nil {
		return 0
	}
	return int(*f)
}

// flexInt64 is the 64-bit variant of flexInt.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(b []byte) error {
	n, err := parseFlexInt(b)
	if err != nil {
		return err
	}
	*f = flexInt64(n)
	return nil
}

func (f *flexInt64) Int64() int64 {
	if f == nil {
		return 0
	}
	return int64(*f)
}

func parseFlexInt(b []byte) (int64, error) {
	raw := strings.TrimSpace(strings.Trim(string(b), `"`))
	if raw == "" || raw == "null" {
		return 0, nil
	}
	return strconv.ParseInt(raw, 10, 64)
}

type subscriptionPlan struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Price        string `json:"price"`
	DurationDays int    `json:"duration_days"`
	GroupName    string `json:"group_name"`
	TokenQuota   int64  `json:"token_quota"`
	SortOrder    int    `json:"sort_order"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
}

type userSubscription struct {
	ID        string `json:"id"`
	PlanID    string `json:"plan_id"`
	PlanName  string `json:"plan_name"`
	Price     string `json:"price"`
	StartsAt  string `json:"starts_at"`
	ExpiresAt string `json:"expires_at"`
	Status    string `json:"status"`
	AutoRenew bool   `json:"auto_renew"`
}

func scanSubscriptionPlan(row interface{ Scan(...any) error }) (subscriptionPlan, error) {
	var p subscriptionPlan
	var id uuid.UUID
	var price decimal.Decimal
	var createdAt time.Time
	if err := row.Scan(&id, &p.Name, &p.Description, &price, &p.DurationDays, &p.GroupName, &p.TokenQuota, &p.SortOrder, &p.Enabled, &createdAt); err != nil {
		return p, err
	}
	p.ID = id.String()
	p.Price = price.String()
	p.CreatedAt = createdAt.Format(time.RFC3339)
	return p, nil
}

// --- Admin: plan CRUD ---

func HandleListSubscriptionPlans(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT id, name, description, price, duration_days, COALESCE(group_name, ''), token_quota, sort_order, enabled, created_at
			 FROM subscription_plans ORDER BY sort_order DESC, created_at DESC`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list plans"})
			return
		}
		defer rows.Close()
		plans := []subscriptionPlan{}
		for rows.Next() {
			p, err := scanSubscriptionPlan(rows)
			if err == nil {
				plans = append(plans, p)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": plans, "total": len(plans)})
	}
}

func HandleCreateSubscriptionPlan(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		var req struct {
			Name         string    `json:"name"`
			Description  string    `json:"description"`
			Price        string    `json:"price"`
			DurationDays flexInt   `json:"duration_days"`
			GroupName    string    `json:"group_name"`
			TokenQuota   flexInt64 `json:"token_quota"`
			SortOrder    flexInt   `json:"sort_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		price, err := decimal.NewFromString(req.Price)
		if err != nil || price.LessThanOrEqual(decimal.Zero) || req.DurationDays.Int() <= 0 || req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, positive price and duration_days are required"})
			return
		}
		id := uuid.New()
		if _, err := a.Pool.Exec(r.Context(),
			`INSERT INTO subscription_plans (id, name, description, price, duration_days, group_name, token_quota, sort_order, enabled)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, TRUE)`,
			id, req.Name, req.Description, price, req.DurationDays.Int(), req.GroupName, req.TokenQuota.Int64(), req.SortOrder.Int()); err != nil {
			log.Printf("console: create plan: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create plan"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id.String()})
	}
}

func HandleUpdateSubscriptionPlan(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		planID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid plan ID"})
			return
		}
		var req struct {
			Name         *string    `json:"name"`
			Description  *string    `json:"description"`
			Price        *string    `json:"price"`
			DurationDays *flexInt   `json:"duration_days"`
			GroupName    *string    `json:"group_name"`
			TokenQuota   *flexInt64 `json:"token_quota"`
			SortOrder    *flexInt   `json:"sort_order"`
			Enabled      *bool      `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Price != nil {
			if _, err := decimal.NewFromString(*req.Price); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid price"})
				return
			}
		}
		var durationDays *int
		if req.DurationDays != nil {
			v := req.DurationDays.Int()
			durationDays = &v
		}
		var tokenQuota *int64
		if req.TokenQuota != nil {
			v := req.TokenQuota.Int64()
			tokenQuota = &v
		}
		var sortOrder *int
		if req.SortOrder != nil {
			v := req.SortOrder.Int()
			sortOrder = &v
		}
		if _, err := a.Pool.Exec(r.Context(),
			`UPDATE subscription_plans SET
				name = COALESCE($2, name),
				description = COALESCE($3, description),
				price = COALESCE($4::decimal, price),
				duration_days = COALESCE($5, duration_days),
				group_name = COALESCE($6, group_name),
				token_quota = COALESCE($7, token_quota),
				sort_order = COALESCE($8, sort_order),
				enabled = COALESCE($9, enabled),
				updated_at = NOW()
			 WHERE id = $1`,
			planID, req.Name, req.Description, req.Price, durationDays, req.GroupName, tokenQuota, sortOrder, req.Enabled); err != nil {
			log.Printf("console: update plan: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update plan"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func HandleDeleteSubscriptionPlan(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		planID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid plan ID"})
			return
		}
		if _, err := a.Pool.Exec(r.Context(), `DELETE FROM subscription_plans WHERE id = $1`, planID); err != nil {
			log.Printf("console: delete plan: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete plan"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// --- User: list / purchase / my subscriptions ---

func HandleListPlans(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := a.Pool.Query(r.Context(),
			`SELECT id, name, description, price, duration_days, COALESCE(group_name, ''), token_quota, sort_order, enabled, created_at
			 FROM subscription_plans WHERE enabled = TRUE ORDER BY sort_order DESC, created_at DESC`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list plans"})
			return
		}
		defer rows.Close()
		plans := []subscriptionPlan{}
		for rows.Next() {
			p, err := scanSubscriptionPlan(rows)
			if err == nil {
				plans = append(plans, p)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": plans, "total": len(plans)})
	}
}

func HandlePurchaseSubscription(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var req struct {
			PlanID    string `json:"plan_id"`
			AutoRenew bool   `json:"auto_renew"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		planID, err := uuid.Parse(req.PlanID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid plan ID"})
			return
		}
		var planName string
		var price decimal.Decimal
		err = a.Pool.QueryRow(r.Context(),
			`SELECT name, price FROM subscription_plans WHERE id = $1 AND enabled = TRUE`,
			planID).Scan(&planName, &price)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plan not found"})
			return
		}

		wlt, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil || wlt == nil {
			writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "No wallet for this account"})
			return
		}
		idem := "sub:" + uuid.New().String()
		if _, err := a.Wallets.Spend(r.Context(), wlt.ID, price, idem); err != nil {
			if errors.Is(err, wallet.ErrInsufficientBalance) {
				writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "余额不足，请先充值"})
				return
			}
			log.Printf("console: subscription spend: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to deduct balance"})
			return
		}

		expiresAt, err := subscriptionsvc.New(a.Pool).ActivateWithAutoRenew(r.Context(), userID, planID, req.AutoRenew)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to activate subscription"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "plan_name": planName, "price": price.String(),
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	}
}

// HandleCreateSubscriptionOrder creates a payment order for a subscription
// plan (epay Alipay/WeChat); the plan activates on paid callback.
func HandleCreateSubscriptionOrder(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var req struct {
			PlanID    string `json:"plan_id"`
			PayMethod string `json:"pay_method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		planID, err := uuid.Parse(req.PlanID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid plan ID"})
			return
		}
		plan, err := subscriptionsvc.New(a.Pool).FindEnabled(r.Context(), planID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Plan not found"})
			return
		}
		if a.Payment == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Payment is not configured"})
			return
		}
		order, err := a.Payment.CreateSubscriptionOrder(r.Context(), userID, planID, plan.Price, req.PayMethod)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, order)
	}
}

// HandleSetAutoRenew toggles auto-renew consent for the user's active
// subscription(s).
func HandleSetAutoRenew(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if err := subscriptionsvc.New(a.Pool).SetAutoRenew(r.Context(), userID, req.Enabled); err != nil {
			log.Printf("console: set auto renew: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update auto-renew"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": req.Enabled})
	}
}

func HandleMySubscriptions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT id, plan_id, plan_name, price, starts_at, expires_at, status, auto_renew
			 FROM user_subscriptions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50`,
			userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list subscriptions"})
			return
		}
		defer rows.Close()
		subs := []userSubscription{}
		for rows.Next() {
			var s userSubscription
			var id, planID uuid.UUID
			var price decimal.Decimal
			var startsAt, expiresAt time.Time
			if err := rows.Scan(&id, &planID, &s.PlanName, &price, &startsAt, &expiresAt, &s.Status, &s.AutoRenew); err == nil {
				s.ID = id.String()
				s.PlanID = planID.String()
				s.Price = price.String()
				s.StartsAt = startsAt.Format(time.RFC3339)
				s.ExpiresAt = expiresAt.Format(time.RFC3339)
				subs = append(subs, s)
			}
		}
		active := []userSubscription{}
		for _, s := range subs {
			if s.Status == "active" {
				active = append(active, s)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"subscriptions": active, "all_subscriptions": subs})
	}
}

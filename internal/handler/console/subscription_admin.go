package console

import (
	"log"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type adminSubscriptionRow struct {
	ID        string `json:"id"`
	UserEmail string `json:"user_email"`
	PlanName  string `json:"plan_name"`
	Price     string `json:"price"`
	StartsAt  string `json:"starts_at"`
	ExpiresAt string `json:"expires_at"`
	Status    string `json:"status"`
}

// HandleListAllSubscriptions returns all user subscriptions with the owning
// user's email (admin operations view).
func HandleListAllSubscriptions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT us.id, u.email, us.plan_name, us.price, us.starts_at, us.expires_at, us.status
			 FROM user_subscriptions us
			 JOIN users u ON u.id = us.user_id
			 ORDER BY us.created_at DESC LIMIT 200`)
		if err != nil {
			log.Printf("console: list subscriptions: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list subscriptions"})
			return
		}
		defer rows.Close()
		items := []adminSubscriptionRow{}
		for rows.Next() {
			var row adminSubscriptionRow
			var id uuid.UUID
			var price decimal.Decimal
			var startsAt, expiresAt time.Time
			if err := rows.Scan(&id, &row.UserEmail, &row.PlanName, &price, &startsAt, &expiresAt, &row.Status); err == nil {
				row.ID = id.String()
				row.Price = price.String()
				row.StartsAt = startsAt.Format(time.RFC3339)
				row.ExpiresAt = expiresAt.Format(time.RFC3339)
				items = append(items, row)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
	}
}

// HandleCancelSubscription cancels an active subscription (admin, audited).
func HandleCancelSubscription(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		subID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid subscription ID"})
			return
		}
		tag, err := a.Pool.Exec(r.Context(),
			`UPDATE user_subscriptions SET status = 'cancelled', updated_at = NOW()
			 WHERE id = $1 AND status = 'active'`, subID)
		if err != nil {
			log.Printf("console: cancel subscription: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to cancel subscription"})
			return
		}
		if tag.RowsAffected() == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Subscription not found or not active"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

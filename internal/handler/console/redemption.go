package console

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func redemptionCode() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "DTP-" + hex.EncodeToString(b)
}

// HandleCreateRedemption creates N redemption codes (admin).
func HandleCreateRedemption(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		var req struct {
			Amount string `json:"amount"`
			Count  int    `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		amount, err := decimal.NewFromString(req.Amount)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid amount"})
			return
		}
		if req.Count <= 0 || req.Count > 5000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "count must be 1..5000"})
			return
		}
		var adminID uuid.UUID
		if uid, err := jwtutil.UserIDFromContext(r.Context()); err == nil {
			adminID = uid
		}
		created := []string{}
		for i := 0; i < req.Count; i++ {
			code := redemptionCode()
			if _, err := a.Pool.Exec(r.Context(),
				`INSERT INTO redemption_codes (code, amount, created_by) VALUES ($1, $2, $3)`,
				code, amount, adminID); err != nil {
				continue
			}
			created = append(created, code)
		}
		writeJSON(w, http.StatusOK, map[string]any{"created": len(created), "codes": created})
	}
}

// HandleListRedemptions lists redemption codes (admin).
func HandleListRedemptions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT rc.code, rc.amount, rc.status, rc.created_at, rc.used_at, u.email
			 FROM redemption_codes rc
			 LEFT JOIN users u ON u.id = rc.used_by
			 ORDER BY rc.created_at DESC LIMIT 200`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list codes"})
			return
		}
		defer rows.Close()
		type codeRow struct {
			Code        string `json:"code"`
			Amount      string `json:"amount"`
			Status      string `json:"status"`
			CreatedAt   string `json:"created_at"`
			UsedAt      string `json:"used_at,omitempty"`
			UsedByEmail string `json:"used_by_email,omitempty"`
		}
		items := []codeRow{}
		for rows.Next() {
			var c codeRow
			var amount decimal.Decimal
			var created time.Time
			var used *time.Time
			var usedByEmail *string
			if err := rows.Scan(&c.Code, &amount, &c.Status, &created, &used, &usedByEmail); err == nil {
				c.Amount = amount.String()
				c.CreatedAt = created.Format("2006-01-02 15:04:05")
				if used != nil {
					c.UsedAt = used.Format("2006-01-02 15:04:05")
				}
				if usedByEmail != nil {
					c.UsedByEmail = *usedByEmail
				}
				items = append(items, c)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"codes": items})
	}
}

// HandleRedeem redeems a code and credits the authenticated user's wallet.
func HandleRedeem(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Code is required"})
			return
		}
		tx, err := a.Pool.Begin(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start tx"})
			return
		}
		defer tx.Rollback(r.Context())

		var codeID uuid.UUID
		var amount decimal.Decimal
		var status string
		err = tx.QueryRow(r.Context(),
			`SELECT id, amount, status FROM redemption_codes WHERE code = $1 FOR UPDATE`, req.Code).Scan(&codeID, &amount, &status)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Invalid redemption code"})
			return
		}
		if status != "active" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Code already used"})
			return
		}
		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Wallet not found"})
			return
		}
		if _, err := a.Wallets.TopUp(r.Context(), wallet.ID, amount, req.Code); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to credit wallet"})
			return
		}
		if _, err := tx.Exec(r.Context(),
			`UPDATE redemption_codes SET status = 'used', used_by = $1, used_at = NOW() WHERE id = $2`,
			userID, codeID); err != nil {
			log.Printf("console: redeem mark used: %v", err)
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "amount": amount.String()})
	}
}

package console

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
)

// statementZone is the billing timezone (Asia/Shanghai): monthly statements
// use GMT+8 civil-month boundaries, matching the pricing peak windows.
var statementZone = time.FixedZone("Asia/Shanghai", 8*3600)

type modelCostRow struct {
	Model string `json:"model"`
	Cost  string `json:"cost"`
	Count int    `json:"count"`
}

type monthlyStatement struct {
	Year        int            `json:"year"`
	Month       int            `json:"month"`
	TotalCost   string         `json:"total_cost"`
	TotalTopup  string         `json:"total_topup"`
	ChargeCount int            `json:"charge_count"`
	ByModel     []modelCostRow `json:"by_model"`
}

// HandleMonthlyStatement returns the authenticated user's billing statement
// for a GMT+8 civil month (default: current month): total model spend, top-up
// inflow and a per-model cost breakdown. Amounts are decimal strings.
func HandleMonthlyStatement(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		now := time.Now().In(statementZone)
		year := now.Year()
		month := int(now.Month())
		if v := r.URL.Query().Get("year"); v != "" {
			year, err = strconv.Atoi(v)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid year"})
				return
			}
		}
		if v := r.URL.Query().Get("month"); v != "" {
			month, err = strconv.Atoi(v)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid month"})
				return
			}
		}
		if year < 2000 || year > 2100 || month < 1 || month > 12 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid year or month"})
			return
		}

		start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, statementZone)
		end := start.AddDate(0, 1, 0)

		var totalCost, chargeCount string
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(final_cost), 0), COUNT(*) FROM usage_logs
			 WHERE user_id = $1 AND status = 'completed' AND created_at >= $2 AND created_at < $3`,
			userID, start, end).Scan(&totalCost, &chargeCount); err != nil {
			log.Printf("console: statement cost: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load statement"})
			return
		}
		count, _ := strconv.Atoi(chargeCount)

		var totalTopup string
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(wt.amount), 0) FROM wallet_transactions wt
			 JOIN wallets w ON w.id = wt.wallet_id
			 WHERE w.user_id = $1 AND wt.tx_type = 'topup' AND wt.created_at >= $2 AND wt.created_at < $3`,
			userID, start, end).Scan(&totalTopup); err != nil {
			log.Printf("console: statement topup: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load statement"})
			return
		}

		rows, err := a.Pool.Query(r.Context(),
			`SELECT public_model_code, COALESCE(SUM(final_cost), 0), COUNT(*) FROM usage_logs
			 WHERE user_id = $1 AND status = 'completed' AND created_at >= $2 AND created_at < $3
			 GROUP BY public_model_code ORDER BY SUM(final_cost) DESC, public_model_code`,
			userID, start, end)
		if err != nil {
			log.Printf("console: statement by model: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load statement"})
			return
		}
		defer rows.Close()

		byModel := make([]modelCostRow, 0)
		for rows.Next() {
			var m modelCostRow
			var cnt int
			if err := rows.Scan(&m.Model, &m.Cost, &cnt); err != nil {
				log.Printf("console: statement row: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load statement"})
				return
			}
			m.Count = cnt
			m.Cost = trimDecimalPrice(m.Cost)
			byModel = append(byModel, m)
		}
		if err := rows.Err(); err != nil {
			log.Printf("console: statement rows: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load statement"})
			return
		}

		writeJSON(w, http.StatusOK, monthlyStatement{
			Year:        year,
			Month:       month,
			TotalCost:   trimDecimalPrice(totalCost),
			TotalTopup:  trimDecimalPrice(totalTopup),
			ChargeCount: count,
			ByModel:     byModel,
		})
	}
}

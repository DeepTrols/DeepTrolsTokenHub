package console

import (
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/shopspring/decimal"
)

type costSummaryResponse struct {
	Model        string `json:"model"`
	RequestCount int64  `json:"request_count"`
	FinalCost    string `json:"final_cost"`    // 售出总额（客户支付）
	UpstreamCost string `json:"upstream_cost"` // 上游成本（进货）
	Profit       string `json:"profit"`        // 利润 = final - upstream
	ProfitMargin string `json:"profit_margin"` // 利润率 %
}

// HandleCostSummary returns per-model revenue/cost/profit aggregation.
// Profit = final_cost (售价) - upstream_cost (上游成本). Admin only.
func HandleCostSummary(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		// Optional time range filter: ?from=YYYY-MM-DD&to=YYYY-MM-DD
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		where := ""
		var args []interface{}
		if from != "" {
			where = " WHERE ul.created_at >= $1"
			args = append(args, from+"T00:00:00Z")
		}
		if to != "" {
			if where == "" {
				where = " WHERE ul.created_at < $1"
			} else {
				where += " AND ul.created_at < $2"
			}
			args = append(args, to+"T23:59:59Z")
		}

		query := `SELECT ul.public_model_code,
				COUNT(*),
				COALESCE(SUM(ul.final_cost), 0),
				COALESCE(SUM(ul.upstream_cost), 0)
			 FROM usage_logs ul` + where +
			` GROUP BY ul.public_model_code ORDER BY 3 DESC`

		rows, err := a.Pool.Query(r.Context(), query, args...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query cost summary"})
			return
		}
		defer rows.Close()

		var summary []costSummaryResponse
		for rows.Next() {
			var c costSummaryResponse
			var finalCost, upstreamCost string
			if err := rows.Scan(&c.Model, &c.RequestCount, &finalCost, &upstreamCost); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read cost summary"})
				return
			}
			c.FinalCost = finalCost
			c.UpstreamCost = upstreamCost
			c.Profit = formatProfit(finalCost, upstreamCost)
			c.ProfitMargin = formatMargin(finalCost, upstreamCost)
			summary = append(summary, c)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate cost summary"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":         summary,
			"total":        len(summary),
			"generated_at": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// formatProfit returns profit = final - upstream as a decimal string.
func formatProfit(finalCost, upstreamCost string) string {
	f, err1 := decimal.NewFromString(finalCost)
	u, err2 := decimal.NewFromString(upstreamCost)
	if err1 != nil || err2 != nil {
		return "0"
	}
	return f.Sub(u).String()
}

// formatMargin returns profit margin as a percentage string (e.g. "65.5%").
func formatMargin(finalCost, upstreamCost string) string {
	f, err1 := decimal.NewFromString(finalCost)
	u, err2 := decimal.NewFromString(upstreamCost)
	if err1 != nil || err2 != nil || f.IsZero() {
		return "0%"
	}
	margin := f.Sub(u).Div(f).Mul(decimal.NewFromInt(100))
	return margin.StringFixed(1) + "%"
}

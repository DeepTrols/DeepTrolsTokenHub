package console

import (
	"net/http"

	"github.com/deeptrols/api/internal/app"
)

type userLedgerRow struct {
	ID           string          `json:"id"`
	Email        string          `json:"email"`
	DisplayName  string          `json:"display_name"`
	Role         string          `json:"role"`
	Status       string          `json:"status"`
	UserType     string          `json:"user_type"`
	TenantID     string          `json:"tenant_id,omitempty"`
	TenantName   string          `json:"tenant_name,omitempty"`
	Balance      string          `json:"balance"`       // 当前可用余额
	Frozen       string          `json:"frozen"`        // 冻结金额
	TotalTopup   string          `json:"total_topup"`   // 累计充值
	TotalSpend   string          `json:"total_spend"`   // 累计消费
	RequestCount int64           `json:"request_count"` // 调用次数
	TotalTokens  int64           `json:"total_tokens"`  // 累计 token
	ModelUsage   []modelUsageRow `json:"model_usage"`   // 每个调用过的模型的聚合（次数/token/费用）
}

type modelUsageRow struct {
	Model  string `json:"model"`
	Calls  int64  `json:"calls"`
	Tokens int64  `json:"tokens"`
	Cost   string `json:"cost"`
}

// HandleUserLedger returns per-user financial and usage ledger (admin only).
// Each row shows balance, cumulative topup/spend, and call/token volume.
func HandleUserLedger(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		rows, err := a.Pool.Query(r.Context(),
			`SELECT u.id, u.email, COALESCE(u.display_name, ''),
			        COALESCE(u.role, 'user'), u.status, u.user_type,
			        COALESCE(tm.tenant_id::text, ''), COALESCE(t.name, ''),
			        COALESCE(w.balance, 0), COALESCE(w.frozen, 0),
			        COALESCE(tx.total_topup, 0),
			        COALESCE(us.total_spend, 0),
			        COALESCE(us.request_count, 0),
			        COALESCE(us.total_tokens, 0)
			 FROM users u
			 LEFT JOIN tenant_memberships tm ON tm.user_id = u.id AND tm.status = 'active'
			 LEFT JOIN tenants t ON t.id = tm.tenant_id
			 LEFT JOIN wallets w ON w.user_id = u.id
			 LEFT JOIN (
			   SELECT w.user_id, COALESCE(SUM(wt.amount), 0) AS total_topup
			   FROM wallet_transactions wt
			   JOIN wallets w ON w.id = wt.wallet_id
			   WHERE wt.tx_type = 'topup'
			   GROUP BY w.user_id
			 ) tx ON tx.user_id = u.id
			 LEFT JOIN (
			   SELECT ul.user_id,
			          COALESCE(SUM(ul.final_cost), 0) AS total_spend,
			          COUNT(*) AS request_count,
			          COALESCE(SUM(CAST(ul.usage_raw->>'total_tokens' AS bigint)), 0) AS total_tokens
			   FROM usage_logs ul
			   GROUP BY ul.user_id
			 ) us ON us.user_id = u.id
			 ORDER BY us.request_count DESC NULLS LAST`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query ledger"})
			return
		}
		defer rows.Close()

		var ledger []userLedgerRow
		for rows.Next() {
			var row userLedgerRow
			// 无调用记录的账号也必须返回空数组（而非 null），保证前端可直接读 .length。
			row.ModelUsage = []modelUsageRow{}
			var balance, frozen, totalTopup, totalSpend string
			if err := rows.Scan(&row.ID, &row.Email, &row.DisplayName, &row.Role, &row.Status,
				&row.UserType, &row.TenantID, &row.TenantName,
				&balance, &frozen, &totalTopup, &totalSpend, &row.RequestCount, &row.TotalTokens); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read ledger"})
				return
			}
			row.Balance = balance
			row.Frozen = frozen
			row.TotalTopup = totalTopup
			row.TotalSpend = totalSpend
			ledger = append(ledger, row)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate ledger"})
			return
		}

		// Per-model aggregated usage per user, one query (no N+1). Every model
		// a user has called appears once with its total calls/tokens/cost,
		// ordered by call count descending.
		modelRows, err := a.Pool.Query(r.Context(),
			`SELECT user_id, public_model_code,
			        COUNT(*) AS calls,
			        COALESCE(SUM(
			          COALESCE(CAST(usage_normalized->>'input_tokens' AS bigint), 0) +
			          COALESCE(CAST(usage_normalized->>'output_tokens' AS bigint), 0)
			        ), 0) AS tokens,
			        COALESCE(SUM(final_cost), 0) AS cost
			 FROM usage_logs
			 GROUP BY user_id, public_model_code
			 ORDER BY user_id, calls DESC, public_model_code`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query model usage"})
			return
		}
		defer modelRows.Close()
		byUser := make(map[string][]modelUsageRow)
		for modelRows.Next() {
			var userID string
			var mu modelUsageRow
			if err := modelRows.Scan(&userID, &mu.Model, &mu.Calls, &mu.Tokens, &mu.Cost); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read model usage"})
				return
			}
			byUser[userID] = append(byUser[userID], mu)
		}
		if err := modelRows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate model usage"})
			return
		}
		for i := range ledger {
			if usage := byUser[ledger[i].ID]; len(usage) > 0 {
				ledger[i].ModelUsage = usage
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"data": ledger, "total": len(ledger)})
	}
}

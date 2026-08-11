package console

import (
	"net/http"

	"github.com/deeptrols/api/internal/app"
)

type userLedgerRow struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"display_name"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	UserType     string `json:"user_type"`
	TenantID     string `json:"tenant_id,omitempty"`
	TenantName   string `json:"tenant_name,omitempty"`
	Balance      string `json:"balance"`       // 当前可用余额
	Frozen       string `json:"frozen"`        // 冻结金额
	TotalTopup   string `json:"total_topup"`   // 累计充值
	TotalSpend   string `json:"total_spend"`   // 累计消费
	RequestCount int64  `json:"request_count"` // 调用次数
	TotalTokens  int64  `json:"total_tokens"`  // 累计 token
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

		writeJSON(w, http.StatusOK, map[string]interface{}{"data": ledger, "total": len(ledger)})
	}
}

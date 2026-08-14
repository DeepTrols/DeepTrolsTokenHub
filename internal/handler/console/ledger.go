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
        COALESCE(prim.tenant_id::text, ''), COALESCE(prim.name, ''),
        COALESCE(w.balance, 0) + COALESCE(emp.emp_balance, 0),
        COALESCE(w.frozen, 0) + COALESCE(emp.emp_frozen, 0),
        COALESCE(tx.total_topup, 0) + COALESCE(emp.emp_topup, 0),
        COALESCE(us.total_spend, 0) + COALESCE(emp.emp_spend, 0),
        COALESCE(us.request_count, 0) + COALESCE(emp.emp_requests, 0),
        COALESCE(us.total_tokens, 0) + COALESCE(emp.emp_tokens, 0)
 FROM users u
 LEFT JOIN (
   -- 每个用户至多归属一个租户：优先展示 owner 身份（兼任其他企业员工的
   -- owner 应归属自己的企业），避免 LEFT JOIN 一成员一行造成重复行。
   SELECT DISTINCT ON (tm.user_id)
          tm.user_id, tm.tenant_id, t.name
   FROM tenant_memberships tm
   JOIN tenants t ON t.id = tm.tenant_id
   WHERE tm.status = 'active'
   ORDER BY tm.user_id,
            CASE tm.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END,
            tm.joined_at
 ) prim ON prim.user_id = u.id
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
 LEFT JOIN (
   -- 企业员工子账号的余额/充值/消费/调用量并入其所属企业（该租户 owner）那一行；
   -- 本身也是任一企业 owner 的账号不并入他企，避免金额被重复统计。
   -- 前提：每个租户仅一个 active owner、一个账号仅归属一个租户（当前注册/子账号流程保证）。
   SELECT own.user_id AS owner_id,
          COALESCE(SUM(ew.balance), 0) AS emp_balance,
          COALESCE(SUM(ew.frozen), 0) AS emp_frozen,
          COALESCE(SUM(etx.total_topup), 0) AS emp_topup,
          COALESCE(SUM(eus.total_spend), 0) AS emp_spend,
          COALESCE(SUM(eus.request_count), 0) AS emp_requests,
          COALESCE(SUM(eus.total_tokens), 0) AS emp_tokens
   FROM tenant_memberships own
   JOIN tenant_memberships emp_m
     ON emp_m.tenant_id = own.tenant_id
    AND emp_m.status = 'active'
    AND emp_m.role IN ('admin', 'member')
   LEFT JOIN (
     SELECT w.user_id, COALESCE(SUM(w.balance), 0) AS balance,
            COALESCE(SUM(w.frozen), 0) AS frozen
     FROM wallets w
     GROUP BY w.user_id
   ) ew ON ew.user_id = emp_m.user_id
   LEFT JOIN (
     SELECT w.user_id, COALESCE(SUM(wt.amount), 0) AS total_topup
     FROM wallet_transactions wt
     JOIN wallets w ON w.id = wt.wallet_id
     WHERE wt.tx_type = 'topup'
     GROUP BY w.user_id
   ) etx ON etx.user_id = emp_m.user_id
   LEFT JOIN (
     SELECT ul.user_id,
            COALESCE(SUM(ul.final_cost), 0) AS total_spend,
            COUNT(*) AS request_count,
            COALESCE(SUM(CAST(ul.usage_raw->>'total_tokens' AS bigint)), 0) AS total_tokens
     FROM usage_logs ul
     GROUP BY ul.user_id
   ) eus ON eus.user_id = emp_m.user_id
   WHERE own.status = 'active'
     AND own.role = 'owner'
     AND NOT EXISTS (
       SELECT 1 FROM tenant_memberships o2
       WHERE o2.user_id = emp_m.user_id
         AND o2.status = 'active'
         AND o2.role = 'owner'
     )
   GROUP BY own.user_id
 ) emp ON emp.owner_id = u.id
 -- 企业员工子账号（active 且 role 为 admin/member）不单独成行；
 -- 兼任 owner 的账号保留，其名下消费随企业账号展示。
 WHERE NOT EXISTS (
   SELECT 1 FROM tenant_memberships vis_emp
   WHERE vis_emp.user_id = u.id
     AND vis_emp.status = 'active'
     AND vis_emp.role IN ('admin', 'member')
 )
 OR EXISTS (
   SELECT 1 FROM tenant_memberships vis_own
   WHERE vis_own.user_id = u.id
     AND vis_own.status = 'active'
     AND vis_own.role = 'owner'
 )
 ORDER BY (COALESCE(us.request_count, 0) + COALESCE(emp.emp_requests, 0)) DESC NULLS LAST`)
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
			`SELECT COALESCE(own.owner_id::text, usage_logs.user_id::text) AS user_id,
       usage_logs.public_model_code,
       COUNT(*) AS calls,
       COALESCE(SUM(
         COALESCE(CAST(usage_normalized->>'input_tokens' AS bigint), 0) +
         COALESCE(CAST(usage_normalized->>'output_tokens' AS bigint), 0)
       ), 0) AS tokens,
       COALESCE(SUM(final_cost), 0) AS cost
 FROM usage_logs
 LEFT JOIN (
   -- 企业员工子账号的模型调用归并到其所属企业的 owner 名下；
   -- 本身也是任一企业 owner 的账号不并入他企，避免重复统计。
   SELECT emp_m.user_id AS employee_id, own2.user_id AS owner_id
   FROM tenant_memberships emp_m
   JOIN tenant_memberships own2
     ON own2.tenant_id = emp_m.tenant_id
    AND own2.status = 'active'
    AND own2.role = 'owner'
   WHERE emp_m.status = 'active'
     AND emp_m.role IN ('admin', 'member')
     AND NOT EXISTS (
       SELECT 1 FROM tenant_memberships o2
       WHERE o2.user_id = emp_m.user_id
         AND o2.status = 'active'
         AND o2.role = 'owner'
     )
 ) own ON own.employee_id = usage_logs.user_id
 GROUP BY COALESCE(own.owner_id::text, usage_logs.user_id::text), usage_logs.public_model_code
 ORDER BY COALESCE(own.owner_id::text, usage_logs.user_id::text), calls DESC, public_model_code`)
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

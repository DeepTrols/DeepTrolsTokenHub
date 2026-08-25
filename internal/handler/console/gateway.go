package console

import (
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
)

type gatewayHealthRow struct {
	ChannelID        string  `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	ModelCode        string  `json:"model_code"`
	PoolType         string  `json:"pool_type"`
	HealthScore      int     `json:"health_score"`
	HealthStatus     string  `json:"health_status"`
	ChannelStatus    string  `json:"channel_status"`
	Strategy         string  `json:"strategy"`
	StickySession    bool    `json:"sticky_session"`
	Weight           int     `json:"weight"`
	InstanceID       *string `json:"instance_id"`
	BaseURL          *string `json:"base_url"`
	CurrentLoad      *int    `json:"current_load"`
	ConcurrencyLimit *int    `json:"concurrency_limit"`
	CooldownUntil    *string `json:"cooldown_until"`
	LastCheckedAt    *string `json:"last_checked_at"`
}

// HandleGatewayHealth returns channels with their instances and live health
// signals (load, concurrency limit, cooldown) for the gateway health view.
func HandleGatewayHealth(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT c.id::text, c.name, COALESCE(m.code, ''), c.pool_type,
			        c.health_score, c.health_status, c.status, c.strategy, c.sticky_session, c.weight,
			        ci.id::text, ci.base_url, ci.current_load, ci.concurrency_limit,
			        ci.cooldown_until, ci.last_checked_at
			 FROM channels c
			 LEFT JOIN models m ON m.id = c.model_id
			 LEFT JOIN channel_instances ci ON ci.channel_id = c.id
			 ORDER BY c.name ASC, ci.created_at ASC`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query gateway health"})
			return
		}
		defer rows.Close()

		out := make([]gatewayHealthRow, 0, 64)
		for rows.Next() {
			var row gatewayHealthRow
			var instanceID, baseURL *string
			var currentLoad, concurrencyLimit *int
			var cooldownUntil, lastCheckedAt *time.Time
			if err := rows.Scan(&row.ChannelID, &row.ChannelName, &row.ModelCode, &row.PoolType,
				&row.HealthScore, &row.HealthStatus, &row.ChannelStatus, &row.Strategy,
				&row.StickySession, &row.Weight,
				&instanceID, &baseURL, &currentLoad, &concurrencyLimit,
				&cooldownUntil, &lastCheckedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read gateway health"})
				return
			}
			row.InstanceID = instanceID
			row.BaseURL = baseURL
			row.CurrentLoad = currentLoad
			row.ConcurrencyLimit = concurrencyLimit
			if cooldownUntil != nil {
				s := cooldownUntil.Format(time.RFC3339)
				row.CooldownUntil = &s
			}
			if lastCheckedAt != nil {
				s := lastCheckedAt.Format(time.RFC3339)
				row.LastCheckedAt = &s
			}
			out = append(out, row)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate gateway health"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

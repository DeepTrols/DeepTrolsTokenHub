package console

import (
	"net/http"

	"github.com/deeptrols/api/internal/app"
)

type auditLogResponse struct {
	ID           string `json:"id"`
	ActorID      string `json:"actor_id,omitempty"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// HandleListAuditLogs returns the most recent audit log entries (admin only).
func HandleListAuditLogs(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		limit, offset := parsePagination(r)

		rows, err := a.Pool.Query(r.Context(),
			`SELECT id, actor_id, action, resource_type, resource_id, ip_address, created_at
			 FROM audit_logs
			 ORDER BY created_at DESC
			 LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query audit logs"})
			return
		}
		defer rows.Close()

		var entries []auditLogResponse
		for rows.Next() {
			var e auditLogResponse
			var actorID, resourceID *string
			var createdAt string
			if err := rows.Scan(&e.ID, &actorID, &e.Action, &e.ResourceType, &resourceID, &e.IPAddress, &createdAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read audit log"})
				return
			}
			e.CreatedAt = createdAt
			if actorID != nil {
				e.ActorID = *actorID
			}
			if resourceID != nil {
				e.ResourceID = *resourceID
			}
			entries = append(entries, e)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate audit logs"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": entries, "total": len(entries),
		})
	}
}

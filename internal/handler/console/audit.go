package console

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
)

type auditLogResponse struct {
	ID           string          `json:"id"`
	ActorType    string          `json:"actor_type"`
	ActorEmail   string          `json:"actor_email"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	NewValue     json.RawMessage `json:"new_value"`
	Reason       string          `json:"reason"`
	IPAddress    string          `json:"ip_address"`
	CreatedAt    string          `json:"created_at"`
}

// HandleListAuditLogs returns the most recent admin audit entries. The
// platform writes audit_logs for every admin mutation and content-policy
// block; this endpoint exposes them for the audit page.
func HandleListAuditLogs(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT al.id, al.actor_type, COALESCE(u.email, ''), al.action,
			        al.resource_type, COALESCE(al.resource_id::text, ''),
			        COALESCE(al.new_value::text, '{}'), COALESCE(al.reason, ''),
			        COALESCE(al.ip_address, ''), al.created_at
			 FROM audit_logs al
			 LEFT JOIN users u ON u.id = al.actor_id
			 ORDER BY al.created_at DESC
			 LIMIT 200`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list audit logs"})
			return
		}
		defer rows.Close()

		out := make([]auditLogResponse, 0, 100)
		for rows.Next() {
			var item auditLogResponse
			var newValue string
			var createdAt time.Time
			if err := rows.Scan(&item.ID, &item.ActorType, &item.ActorEmail, &item.Action,
				&item.ResourceType, &item.ResourceID, &newValue, &item.Reason,
				&item.IPAddress, &createdAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read audit log"})
				return
			}
			item.NewValue = json.RawMessage(newValue)
			item.CreatedAt = createdAt.Format(time.RFC3339)
			out = append(out, item)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate audit logs"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

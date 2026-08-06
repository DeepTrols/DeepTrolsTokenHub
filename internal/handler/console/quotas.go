package console

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type quotaPoolResponse struct {
	ID              string `json:"id"`
	TenantID        string `json:"tenant_id"`
	TenantName      string `json:"tenant_name"`
	ModelID         string `json:"model_id"`
	ModelCode       string `json:"model_code"`
	ModelName       string `json:"model_name"`
	Dimension       string `json:"dimension"`
	TotalAmount     int64  `json:"total_amount"`
	AllocatedAmount int64  `json:"allocated_amount"`
	UsedAmount      int64  `json:"used_amount"`
	UnitName        string `json:"unit_name"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// HandleListQuotaPools returns all quota pools with tenant and model info.
func HandleListQuotaPools(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		rows, err := a.Pool.Query(ctx,
			`SELECT qp.id, qp.tenant_id, COALESCE(t.name, ''), qp.model_id,
			        COALESCE(m.code, ''), COALESCE(m.display_name, ''),
			        qp.dimension, qp.total_amount, qp.allocated_amount,
			        qp.used_amount, qp.unit_name, qp.created_at, qp.updated_at
			 FROM quota_pools qp
			 LEFT JOIN tenants t ON qp.tenant_id = t.id
			 LEFT JOIN models m ON qp.model_id = m.id
			 ORDER BY qp.created_at DESC`,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list quota pools"})
			return
		}
		defer rows.Close()

		pools := make([]quotaPoolResponse, 0)
		for rows.Next() {
			var p quotaPoolResponse
			var createdAt, updatedAt time.Time
			if err := rows.Scan(&p.ID, &p.TenantID, &p.TenantName, &p.ModelID,
				&p.ModelCode, &p.ModelName, &p.Dimension, &p.TotalAmount,
				&p.AllocatedAmount, &p.UsedAmount, &p.UnitName,
				&createdAt, &updatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read quota pool"})
				return
			}
			p.CreatedAt = createdAt.Format(time.RFC3339)
			p.UpdatedAt = updatedAt.Format(time.RFC3339)
			pools = append(pools, p)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate quota pools"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  pools,
			"total": len(pools),
		})
	}
}

// HandleCreateQuotaPool creates a new quota pool.
func HandleCreateQuotaPool(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TenantID    *string `json:"tenant_id"`
			ModelID     *string `json:"model_id"`
			Dimension   string  `json:"dimension"`
			TotalAmount int64   `json:"total_amount"`
			UnitName    string  `json:"unit_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.TotalAmount <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "total_amount must be positive"})
			return
		}
		if req.Dimension == "" {
			req.Dimension = "token"
		}
		if req.UnitName == "" {
			req.UnitName = "token"
		}

		id := uuid.New()
		now := time.Now()
		_, err := a.Pool.Exec(r.Context(),
			`INSERT INTO quota_pools (id, tenant_id, model_id, dimension, total_amount, allocated_amount, used_amount, unit_name, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, 0, 0, $6, $7, $7)`,
			id, nullUUID(req.TenantID), nullUUID(req.ModelID), req.Dimension, req.TotalAmount, req.UnitName, now,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create quota pool"})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id.String()})
	}
}

// HandleAllocateQuota allocates quota from a pool to a user.
func HandleAllocateQuota(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid pool ID"})
			return
		}

		var req struct {
			UserID string `json:"user_id"`
			Amount int64  `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Amount <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "amount must be positive"})
			return
		}
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		// Check pool exists and has remaining capacity.
		var totalAmount, allocatedAmount int64
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT total_amount, allocated_amount FROM quota_pools WHERE id=$1`, poolID,
		).Scan(&totalAmount, &allocatedAmount); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Quota pool not found"})
			return
		}
		if allocatedAmount+req.Amount > totalAmount {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Insufficient pool capacity"})
			return
		}

		allocID := uuid.New()
		now := time.Now()
		if _, err := a.Pool.Exec(r.Context(),
			`INSERT INTO quota_allocations (id, pool_id, user_id, allocated_amount, used_amount, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 0, $5, $5)
			 ON CONFLICT (pool_id, user_id) DO UPDATE SET allocated_amount = quota_allocations.allocated_amount + $4, updated_at = $5`,
			allocID, poolID, userID, req.Amount, now,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create allocation"})
			return
		}

		// Update pool allocated counter.
		a.Pool.Exec(r.Context(),
			`UPDATE quota_pools SET allocated_amount = allocated_amount + $1, updated_at = NOW() WHERE id = $2`,
			req.Amount, poolID,
		)
		writeJSON(w, http.StatusCreated, map[string]string{"id": allocID.String()})
	}
}

// HandleQuotaLedger returns ledger entries for a given allocation.
func HandleQuotaLedger(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allocID := r.URL.Query().Get("allocation_id")
		if allocID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "allocation_id is required"})
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT id, allocation_id, action, amount, balance_after, idempotency_key, created_at
			 FROM quota_ledger WHERE allocation_id=$1 ORDER BY created_at DESC LIMIT 100`, allocID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Query failed"})
			return
		}
		defer rows.Close()

		type entry struct {
			ID             string `json:"id"`
			AllocationID   string `json:"allocation_id"`
			Action         string `json:"action"`
			Amount         int64  `json:"amount"`
			BalanceAfter   int64  `json:"balance_after"`
			IdempotencyKey string `json:"idempotency_key"`
			CreatedAt      string `json:"created_at"`
		}
		entries := make([]entry, 0)
		for rows.Next() {
			var e entry
			var t time.Time
			rows.Scan(&e.ID, &e.AllocationID, &e.Action, &e.Amount, &e.BalanceAfter, &e.IdempotencyKey, &t)
			e.CreatedAt = t.Format(time.RFC3339)
			entries = append(entries, e)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"data": entries})
	}
}

func nullUUID(s *string) interface{} {
	if s == nil || *s == "" {
		return nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil
	}
	return id
}

package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/repository/quota"
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
			`SELECT qp.id, COALESCE(qp.tenant_id::text, ''), COALESCE(t.name, ''),
			        COALESCE(qp.model_id::text, ''), COALESCE(m.code, ''), COALESCE(m.display_name, ''),
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
		// tenant_id is NOT NULL in the schema; every pool belongs to one tenant.
		// Reject a missing/invalid value here so the caller gets a 400 instead of
		// an opaque 500 from the insert.
		if req.TenantID == nil || *req.TenantID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant_id is required"})
			return
		}
		if _, err := uuid.Parse(*req.TenantID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant_id"})
			return
		}
		// A model_id that is present but not a valid UUID must be rejected here
		// rather than silently becoming a tenant-wide pool (nullUUID would map
		// it to NULL). Quota meant for one model must not silently apply to
		// every model under the tenant.
		if req.ModelID != nil && *req.ModelID != "" {
			if _, err := uuid.Parse(*req.ModelID); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model_id"})
				return
			}
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

// HandleUpdateQuotaPool updates the editable fields of a quota pool: total
// amount, unit, and dimension. The pool's tenant/model scope is immutable — a
// pool meant for one model must never silently become tenant-wide by editing
// its scope.
func HandleUpdateQuotaPool(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid pool ID"})
			return
		}

		var req struct {
			TotalAmount int64  `json:"total_amount"`
			UnitName    string `json:"unit_name"`
			Dimension   string `json:"dimension"`
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

		pool, err := a.Quotas.UpdatePool(r.Context(), poolID, req.TotalAmount, req.UnitName, req.Dimension)
		if err != nil {
			if errors.Is(err, quota.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Quota pool not found"})
				return
			}
			if errors.Is(err, quota.ErrConstraintViolation) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "total_amount cannot be below the already allocated amount"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update quota pool"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":               pool.ID.String(),
			"total_amount":     pool.TotalAmount,
			"allocated_amount": pool.AllocatedAmount,
			"used_amount":      pool.UsedAmount,
			"unit_name":        pool.UnitName,
			"dimension":        pool.Dimension,
		})
	}
}

// HandleDeleteQuotaPool permanently removes a quota pool together with its
// allocations and ledger entries.
func HandleDeleteQuotaPool(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		poolID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid pool ID"})
			return
		}

		if err := a.Quotas.DeletePool(r.Context(), poolID); err != nil {
			if errors.Is(err, quota.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Quota pool not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete quota pool"})
			return
		}

		// 200 with a JSON body (not 204): the console frontend's request()
		// helper always reads res.json(), and a bodyless 204 makes it reject,
		// so a successful delete would be reported as an error. Matches
		// HandleDeleteTenant's convention.
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "deleted",
			"id":     poolID.String(),
		})
	}
}

// HandleAllocateQuota allocates quota from a pool to a user. The capacity check,
// allocation upsert, pool counter, and ledger entry all happen atomically in the
// repository, so concurrent allocations can never oversubscribe a pool.
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

		// Derive the idempotency key from (pool, user, amount): a retried request
		// replays the recorded allocation instead of granting quota twice, while
		// a genuinely different allocation (different user, pool, or amount)
		// still goes through. A fresh UUID per call would defeat idempotency.
		key := "admin-allocate:" + poolID.String() + ":" + userID.String() + ":" + strconv.FormatInt(req.Amount, 10)
		alloc, err := a.Quotas.Allocate(r.Context(), poolID, userID, req.Amount, key)
		if err != nil {
			if errors.Is(err, quota.ErrInsufficientQuota) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Insufficient pool capacity"})
				return
			}
			if errors.Is(err, quota.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Quota pool not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create allocation"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"id": alloc.ID.String()})
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
		if _, err := uuid.Parse(allocID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid allocation_id"})
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

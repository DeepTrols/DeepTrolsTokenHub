package console

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/quota"
	"github.com/google/uuid"
)

// teamQuotaAllocation is one sub-account's share of a quota pool.
type teamQuotaAllocation struct {
	UserID    string `json:"user_id"`
	Allocated int64  `json:"allocated"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"`
}

// teamQuotaPool is a quota pool as seen by an enterprise admin, including the
// allocations made against it.
type teamQuotaPool struct {
	ID          string                `json:"id"`
	ModelID     string                `json:"model_id,omitempty"`
	Dimension   string                `json:"dimension"`
	TotalAmount int64                 `json:"total_amount"`
	Allocated   int64                 `json:"allocated"`
	Used        int64                 `json:"used"`
	Remaining   int64                 `json:"remaining"`
	UnitName    string                `json:"unit_name"`
	Allocations []teamQuotaAllocation `json:"allocations"`
}

// HandleListTeamQuotas returns the tenant's quota pools with per-sub-account
// allocations, so an enterprise admin can see what has been handed out and
// what headroom remains (total − allocated).
func HandleListTeamQuotas(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}

		pools, err := a.Quotas.FindPoolsByTenant(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleListTeamQuotas: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list quota pools"})
			return
		}
		allocations, err := a.Quotas.FindAllocationsByTenant(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleListTeamQuotas: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list allocations"})
			return
		}

		byPool := make(map[uuid.UUID][]domain.QuotaAllocation)
		for _, alloc := range allocations {
			byPool[alloc.PoolID] = append(byPool[alloc.PoolID], alloc)
		}

		items := make([]teamQuotaPool, 0, len(pools))
		for _, p := range pools {
			item := teamQuotaPool{
				ID:          p.ID.String(),
				Dimension:   p.Dimension,
				TotalAmount: p.TotalAmount,
				Allocated:   p.AllocatedAmount,
				Used:        p.UsedAmount,
				Remaining:   p.TotalAmount - p.AllocatedAmount,
				UnitName:    p.UnitName,
				Allocations: make([]teamQuotaAllocation, 0),
			}
			if p.ModelID != nil {
				item.ModelID = p.ModelID.String()
			}
			for _, alloc := range byPool[p.ID] {
				item.Allocations = append(item.Allocations, teamQuotaAllocation{
					UserID:    alloc.UserID.String(),
					Allocated: alloc.AllocatedAmount,
					Used:      alloc.UsedAmount,
					Remaining: alloc.Remaining(),
				})
			}
			items = append(items, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{"pools": items})
	}
}

type allocateTeamQuotaRequest struct {
	UserID string `json:"user_id"`
	PoolID string `json:"pool_id"`
	Amount int64  `json:"amount"`
}

// HandleAllocateTeamQuota lets an enterprise admin hand out quota from the
// tenant's pools to a sub-account. The allocation can never exceed the pool's
// remaining headroom (pool total − already allocated); the bound is enforced
// atomically inside the repository under a row lock.
func HandleAllocateTeamQuota(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}

		var req allocateTeamQuotaRequest
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
		poolID, err := uuid.Parse(req.PoolID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid pool ID"})
			return
		}

		ctx := r.Context()

		// The recipient must be an active member of this tenant.
		if m, err := a.Memberships.FindByUserAndTenant(ctx, userID, tenantID); err != nil || m.Status != domain.MembershipStatusActive {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Sub-account not found in this team"})
			return
		}

		// The pool must belong to this tenant.
		pool, err := a.Quotas.FindPool(ctx, poolID)
		if err != nil || pool.TenantID != tenantID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Quota pool not found"})
			return
		}

		// Deterministic idempotency key from (pool, user, amount): a retried
		// request replays the recorded allocation instead of granting quota
		// twice, while a different amount still appends a fresh allocation.
		key := "team-allocate:" + poolID.String() + ":" + userID.String() + ":" + strconv.FormatInt(req.Amount, 10)
		alloc, err := a.Quotas.Allocate(ctx, poolID, userID, req.Amount, key)
		if err != nil {
			if errors.Is(err, quota.ErrInsufficientQuota) {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "Allocation exceeds the pool's remaining capacity",
				})
				return
			}
			if errors.Is(err, quota.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Quota pool not found"})
				return
			}
			log.Printf("HandleAllocateTeamQuota: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to allocate quota"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":        alloc.ID.String(),
			"pool_id":   alloc.PoolID.String(),
			"user_id":   alloc.UserID.String(),
			"allocated": alloc.AllocatedAmount,
			"used":      alloc.UsedAmount,
			"remaining": alloc.Remaining(),
		})
	}
}

// HandleTeamQuotaLedger returns the audit trail for one allocation. The
// allocation must belong to the caller's tenant, so an admin can never read
// another tenant's ledger.
func HandleTeamQuotaLedger(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}

		allocIDStr := r.URL.Query().Get("allocation_id")
		if allocIDStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "allocation_id is required"})
			return
		}
		allocID, err := uuid.Parse(allocIDStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid allocation ID"})
			return
		}

		// The allocation must sit under one of this tenant's pools.
		var poolTenantID uuid.UUID
		err = a.Pool.QueryRow(r.Context(),
			`SELECT p.tenant_id
			 FROM quota_allocations a
			 JOIN quota_pools p ON a.pool_id = p.id
			 WHERE a.id = $1`, allocID,
		).Scan(&poolTenantID)
		if err != nil || poolTenantID != tenantID {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Allocation not found"})
			return
		}

		entries, err := a.Quotas.FindLedgerByAllocation(r.Context(), allocID, 100, 0)
		if err != nil {
			log.Printf("HandleTeamQuotaLedger: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list ledger entries"})
			return
		}

		type entryItem struct {
			ID           string `json:"id"`
			Action       string `json:"action"`
			Amount       int64  `json:"amount"`
			BalanceAfter int64  `json:"balance_after"`
			CreatedAt    string `json:"created_at"`
		}
		items := make([]entryItem, 0, len(entries))
		for _, e := range entries {
			items = append(items, entryItem{
				ID:           e.ID.String(),
				Action:       string(e.Action),
				Amount:       e.Amount,
				BalanceAfter: e.BalanceAfter,
				CreatedAt:    e.CreatedAt.Format(time.RFC3339),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": items})
	}
}

package console

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	walletRepo "github.com/deeptrols/api/internal/repository/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type allocateBalanceRequest struct {
	UserID string `json:"user_id"`
	Amount string `json:"amount"`
}

// HandleAllocateBalance lets an enterprise admin transfer money from their own
// wallet to a team member's wallet. The member then spends that balance
// directly through the gateway (which already charges wallets), so no quota
// pool is involved. The recipient must be an active member of the caller's
// tenant — cross-tenant transfers are rejected.
func HandleAllocateBalance(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}

		var req allocateBalanceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		amount, err := decimal.NewFromString(req.Amount)
		if err != nil || amount.LessThanOrEqual(decimal.Zero) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Amount must be positive"})
			return
		}
		targetID, err := uuid.Parse(req.UserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		ctx := r.Context()

		// The recipient must be an active member of this tenant. Rejecting here
		// keeps transfers strictly same-tenant: an admin can never fund a user
		// outside their own team.
		m, err := a.Memberships.FindByUserAndTenant(ctx, targetID, tenantID)
		if err != nil || m.Status != domain.MembershipStatusActive {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Member not found in this team"})
			return
		}
		// Transferring to yourself is a no-op that would collide on the ledger's
		// (wallet, idempotency) uniqueness; reject it explicitly.
		if targetID == adminID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot transfer balance to yourself"})
			return
		}

		// Both sides are personal wallets (tenant_id IS NULL), created at
		// registration / sub-account provisioning.
		fromWallet, err := a.Wallets.FindByUser(ctx, adminID, nil)
		if err != nil {
			log.Printf("HandleAllocateBalance: from wallet: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read wallet"})
			return
		}
		toWallet, err := a.Wallets.FindByUser(ctx, targetID, nil)
		if err != nil {
			log.Printf("HandleAllocateBalance: to wallet: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read wallet"})
			return
		}

		// Deterministic idempotency key from (admin, member): a retried request
		// replays the recorded transfer instead of moving money twice.
		key := "balance-transfer:" + adminID.String() + ":" + targetID.String()

		transfer, err := a.Wallets.Transfer(ctx, fromWallet.ID, toWallet.ID, amount, key)
		if err != nil {
			if errors.Is(err, walletRepo.ErrInsufficientBalance) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Wallet balance insufficient"})
				return
			}
			log.Printf("HandleAllocateBalance: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to allocate balance"})
			return
		}

		// Re-read both wallets for the latest balances to return to the caller.
		fromAfter, _ := a.Wallets.FindByUser(ctx, adminID, nil)
		toAfter, _ := a.Wallets.FindByUser(ctx, targetID, nil)

		writeJSON(w, http.StatusOK, map[string]any{
			"transaction_id": transfer.ID.String(),
			"from_balance":   fromAfter.Balance.String(),
			"to_balance":     toAfter.Balance.String(),
		})
	}
}

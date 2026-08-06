package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	walletRepo "github.com/deeptrols/api/internal/repository/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type walletResponse struct {
	Balance      string `json:"balance"`
	Frozen       string `json:"frozen"`
	Available    string `json:"available"`
	Currency     string `json:"currency"`
	TotalCharged string `json:"total_charged"`
}

type transactionResponse struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Amount        string `json:"amount"`
	BalanceAfter  string `json:"balance_after"`
	Reference     string `json:"reference"`
	OrderNo       string `json:"order_no"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method"`
	CreatedAt     string `json:"created_at"`
}

func HandleGetWallet(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil {
			if errors.Is(err, walletRepo.ErrNotFound) {
				writeJSON(w, http.StatusOK, walletResponse{
					Balance: "0", Frozen: "0", Available: "0", Currency: "CNY",
				})
				return
			}
			log.Printf("console: wallet lookup error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve wallet"})
			return
		}

		// Compute cumulative consumption: sum of 'charge' transaction amounts.
		// charge amounts are negative (deductions), so negate for a positive total.
		var totalCharged string
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(-SUM(amount), 0) FROM wallet_transactions
			 WHERE wallet_id = $1 AND tx_type = 'charge'`, wallet.ID,
		).Scan(&totalCharged); err != nil {
			log.Printf("console: total_charged query error: %v", err)
			totalCharged = "0"
		}

		writeJSON(w, http.StatusOK, walletResponse{
			Balance:      wallet.Balance.String(),
			Frozen:       wallet.Frozen.String(),
			Available:    wallet.Available().String(),
			Currency:     wallet.Currency,
			TotalCharged: totalCharged,
		})
	}
}

func HandleListTransactions(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil {
			if errors.Is(err, walletRepo.ErrNotFound) {
				writeJSON(w, http.StatusOK, map[string]interface{}{
					"data": []interface{}{}, "total": 0,
				})
				return
			}
			log.Printf("console: wallet lookup error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve wallet"})
			return
		}

		limit, offset := parsePagination(r)
		txs, err := a.Wallets.ListTransactions(r.Context(), wallet.ID, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list transactions"})
			return
		}

		response := make([]transactionResponse, 0, len(txs))
		for _, tx := range txs {
			tr := transactionResponse{
				ID:           tx.ID.String(),
				Type:         string(tx.TxType),
				Amount:       tx.Amount.String(),
				BalanceAfter: tx.BalanceAfter.String(),
				Reference:    tx.ReferenceType,
				CreatedAt:    tx.CreatedAt.Format(time.RFC3339),
			}
			if tx.Metadata != nil {
				if v, ok := tx.Metadata["order_no"].(string); ok {
					tr.OrderNo = v
				}
				if v, ok := tx.Metadata["status"].(string); ok {
					tr.Status = v
				}
				if v, ok := tx.Metadata["payment_method"].(string); ok {
					tr.PaymentMethod = v
				}
			}
			response = append(response, tr)
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": response, "total": len(response),
		})
	}
}

// ============================================================================
// Redemption Code
// ============================================================================

// redeemRequest is the JSON body for POST /wallet/redeem.
type redeemRequest struct {
	Code string `json:"code"`
}

// HandleRedeem validates a redemption code and credits the user's wallet.
// Requires ENABLE_FAKE_PAYMENT=true (demo only); otherwise returns 403.
func HandleRedeem(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.Config.FakePayment {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "兑换码充值未开通"})
			return
		}

		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		var req redeemRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if strings.TrimSpace(req.Code) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "兑换码不能为空"})
			return
		}

		code := strings.TrimSpace(req.Code)

		// Look up the redemption code. For now, accept any well-formed code
		// and grant a fixed bonus amount. Replace with a real redeem_codes table
		// in production.
		amount := redeemCodeValue(code)
		if amount.LessThanOrEqual(decimal.Zero) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的兑换码"})
			return
		}

		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil {
			if errors.Is(err, walletRepo.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Wallet not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve wallet"})
			return
		}

		idempotencyKey := "redeem:" + code + ":" + userID.String()
		tx, err := a.Wallets.TopUp(r.Context(), wallet.ID, amount, idempotencyKey)
		// Store order metadata.
		if err != nil {
			// Duplicate redemption — idempotency key conflict means code already used.
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "该兑换码已被使用"})
				return
			}
			log.Printf("console: redeem error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "兑换失败，请稍后重试"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"amount":        tx.Amount.String(),
				"balance_after": tx.BalanceAfter.String(),
				"message":       fmt.Sprintf("成功兑换 %s CNY", tx.Amount.String()),
			},
		})
	}
}

// redeemCodeValue determines the value of a redemption code.
// In production, this queries a redeem_codes table.
func redeemCodeValue(code string) decimal.Decimal {
	// Simple demo logic: codes starting with "DEEP-" grant 50 CNY, others 10 CNY.
	if strings.HasPrefix(strings.ToUpper(code), "DEEP-") {
		return decimal.NewFromFloat(50)
	}
	if strings.HasPrefix(strings.ToUpper(code), "VIP-") {
		return decimal.NewFromFloat(100)
	}
	// Accept any code with at least 6 chars for demo purposes.
	if len(code) >= 6 {
		return decimal.NewFromFloat(10)
	}
	return decimal.Zero
}

// ============================================================================
// Invitation Rewards
// ============================================================================

type inviteResponse struct {
	InviteCode    string `json:"invite_code"`
	TotalRewards  string `json:"total_rewards"`
	ReferralCount int    `json:"referral_count"`
	Message       string `json:"message,omitempty"`
}

// HandleGetInvite returns the current user's invite code and accumulated rewards.
func HandleGetInvite(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		// Derive invite code from user ID (first 8 chars, uppercased).
		inviteCode := strings.ToUpper(userID.String()[:8])

		// Sum invite-reward transactions for this user's wallet.
		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		var totalRewards string
		var referralCount int
		if err == nil {
			_ = a.Pool.QueryRow(r.Context(),
				`SELECT COALESCE(SUM(amount), 0) FROM wallet_transactions
				 WHERE wallet_id = $1 AND tx_type = 'topup' AND reference_type = 'invite_reward'`,
				wallet.ID,
			).Scan(&totalRewards)
			_ = a.Pool.QueryRow(r.Context(),
				`SELECT COUNT(*) FROM wallet_transactions
				 WHERE wallet_id = $1 AND tx_type = 'topup' AND reference_type = 'invite_reward'`,
				wallet.ID,
			).Scan(&referralCount)
		}
		if totalRewards == "" {
			totalRewards = "0"
		}

		writeJSON(w, http.StatusOK, inviteResponse{
			InviteCode:    inviteCode,
			TotalRewards:  totalRewards,
			ReferralCount: referralCount,
		})
	}
}

// HandleTransferRewards moves accumulated invite rewards into the main balance.
func HandleTransferRewards(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Wallet not found"})
			return
		}

		// Sum all invite_reward transactions.
		var totalRewards string
		err = a.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(amount), 0) FROM wallet_transactions
			 WHERE wallet_id = $1 AND tx_type = 'topup' AND reference_type = 'invite_reward'`,
			wallet.ID,
		).Scan(&totalRewards)
		if err != nil || totalRewards == "0" || totalRewards == "0.000" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "没有可转入的返利配额"})
			return
		}

		rewardAmount, _ := decimal.NewFromString(totalRewards)
		idempotencyKey := "xfer_reward:" + userID.String() + ":" + time.Now().UTC().Format("2006-01-02")

		// Create a topup transaction to move rewards to main balance.
		tx, err := a.Wallets.TopUp(r.Context(), wallet.ID, rewardAmount, idempotencyKey)
		// Store order metadata.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"amount":        tx.Amount.String(),
				"balance_after": tx.BalanceAfter.String(),
				"message":       fmt.Sprintf("成功转入 %s CNY 到主余额", tx.Amount.String()),
			},
		})
	}
}

func parsePagination(r *http.Request) (limit, offset int) {
	limit = 20
	offset = 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 100 {
				parsed = 100
			}
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	return
}

// HandleTopUp credits the authenticated user's wallet. Amounts are validated
// with decimal (no floats for money). An idempotency key from the request
// header (X-Request-ID or X-Idempotency-Key) prevents duplicate credits on retry.
// Requires ENABLE_FAKE_PAYMENT=true (demo only); otherwise returns 403.
func HandleTopUp(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.Config.FakePayment {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "在线充值未开通"})
			return
		}

		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		var req struct {
			Amount        string `json:"amount"`
			PaymentMethod string `json:"payment_method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		amount, err := decimal.NewFromString(req.Amount)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid amount"})
			return
		}
		if amount.LessThanOrEqual(decimal.Zero) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Amount must be positive"})
			return
		}

		wallet, err := a.Wallets.FindByUser(r.Context(), userID, nil)
		if err != nil {
			if errors.Is(err, walletRepo.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Wallet not found"})
				return
			}
			log.Printf("console: wallet lookup error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to retrieve wallet"})
			return
		}

		idempotencyKey := r.Header.Get("X-Idempotency-Key")
		if idempotencyKey == "" {
			idempotencyKey = r.Header.Get("X-Request-ID")
		}
		if idempotencyKey == "" {
			idempotencyKey = uuid.New().String()
		}

		tx, err := a.Wallets.TopUp(r.Context(), wallet.ID, amount, idempotencyKey)
		// Store order metadata.
		pm := req.PaymentMethod
		if pm == "" {
			pm = "alipay"
		}
		orderNo := fmt.Sprintf("TXN%s%04d", time.Now().Format("20060102150405"), time.Now().Nanosecond()%10000)
		a.Pool.Exec(r.Context(), `UPDATE wallet_transactions SET metadata = $1 WHERE id = $2`, fmt.Sprintf(`{"order_no":"%s","status":"success","payment_method":"%s"}`, orderNo, pm), tx.ID)
		if err != nil {
			log.Printf("console: topup error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to credit wallet"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"transaction_id": tx.ID.String(),
				"type":           string(tx.TxType),
				"amount":         tx.Amount.String(),
				"balance_after":  tx.BalanceAfter.String(),
			},
		})
	}
}

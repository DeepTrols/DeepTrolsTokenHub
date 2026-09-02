package console

import (
	"context"
	"fmt"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// currencyCNY is the platform's only supported wallet currency.
const currencyCNY = "CNY"

// Demo-faucet signup bonuses, granted only when ENABLE_FAKE_PAYMENT=true.
// Production runs with zero bonuses; the amounts live here (not at the call
// sites) so the money figures have exactly one home.
var (
	// SignupBonusUser is the signup bonus for self-registered accounts
	// (email registration and OAuth first-login provisioning).
	SignupBonusUser = decimal.NewFromInt(1000)
	// SignupBonusAdmin is the signup bonus for the bootstrap admin account.
	SignupBonusAdmin = decimal.NewFromInt(10000)
)

// SignupBonusKey namespaces the signup-bonus idempotency key per user so a
// replayed provision can never double-credit the bonus.
func SignupBonusKey(userID uuid.UUID) string {
	return "signup-bonus:" + userID.String()
}

// ProvisionUserWallet creates a zero-balance wallet for a new user through
// the wallet repository, then grants a positive signup bonus through an
// idempotent TopUp so every coin granted has a matching wallet_transactions
// row (B2 fix: no bare balance writes). A zero bonus writes no ledger row
// because no money moved. Returns the provisioned wallet with its final
// balance.
func ProvisionUserWallet(ctx context.Context, wallets wallet.Repository, userID uuid.UUID, bonus decimal.Decimal) (*domain.Wallet, error) {
	w := domain.NewWallet(userID, nil, currencyCNY)
	if err := wallets.Create(ctx, &w); err != nil {
		return nil, fmt.Errorf("create wallet for user %s: %w", userID, err)
	}
	if bonus.LessThanOrEqual(decimal.Zero) {
		return &w, nil
	}
	if _, err := wallets.TopUp(ctx, w.ID, bonus, SignupBonusKey(userID)); err != nil {
		return nil, fmt.Errorf("grant signup bonus to user %s: %w", userID, err)
	}
	w.Balance = w.Balance.Add(bonus)
	w.Version++
	return &w, nil
}

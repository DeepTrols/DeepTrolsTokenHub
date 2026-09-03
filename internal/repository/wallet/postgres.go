package wallet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// PostgresRepository implements Repository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

// errInsufficientBalance is returned when the available balance is too low.
// Aliased to the exported sentinel so callers can use errors.Is.
var errInsufficientBalance = ErrInsufficientBalance

// FindByUser retrieves a wallet by user and tenant.
func (r *PostgresRepository) FindByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
	const query = `
		SELECT id, user_id, tenant_id, balance, frozen, currency, version, created_at, updated_at
		FROM wallets
		WHERE user_id = $1 AND tenant_id IS NOT DISTINCT FROM $2
	`
	return scanWallet(r.pool.QueryRow(ctx, query, userID, tenantID))
}

// FindByID retrieves a wallet by its ID.
func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	const query = `
		SELECT id, user_id, tenant_id, balance, frozen, currency, version, created_at, updated_at
		FROM wallets
		WHERE id = $1
	`
	return scanWallet(r.pool.QueryRow(ctx, query, id))
}

// Create inserts a new wallet.
func (r *PostgresRepository) Create(ctx context.Context, wallet *domain.Wallet) error {
	const query = `
		INSERT INTO wallets (id, user_id, tenant_id, balance, frozen, currency, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.pool.Exec(ctx, query,
		wallet.ID, wallet.UserID, wallet.TenantID,
		wallet.Balance, wallet.Frozen, wallet.Currency,
		wallet.Version, wallet.CreatedAt, wallet.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("wallet create: %w", err)
	}
	return nil
}

// Reserve atomically freezes an amount using optimistic locking.
// Returns the created wallet_transaction, or the existing one if idempotent.
func (r *PostgresRepository) Reserve(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wallet reserve begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Check for existing idempotent transaction (scoped to this wallet so
	// identical keys from different users can never resolve to the same tx).
	existing, err := findIdempotentTx(ctx, tx, walletID, idempotencyKey)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("wallet reserve commit: %w", cerr)
		}
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("wallet reserve idempotent check: %w", err)
	}

	// 2. Lock the wallet row for update.
	var balance, frozen decimal.Decimal
	var balanceStr, frozenStr string
	var version int64
	const lockQuery = `
		SELECT balance, frozen, version FROM wallets WHERE id = $1 FOR UPDATE
	`
	lockRow := tx.QueryRow(ctx, lockQuery, walletID)
	if err := lockRow.Scan(&balanceStr, &frozenStr, &version); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("wallet reserve: wallet not found: %w", err)
		}
		return nil, fmt.Errorf("wallet reserve lock: %w", err)
	}
	balance = parseDecimalStr(balanceStr)
	frozen = parseDecimalStr(frozenStr)

	// 3. Check available balance.
	available := balance.Sub(frozen)
	if available.LessThan(amount) {
		return nil, fmt.Errorf("%w: available=%s required=%s", errInsufficientBalance, available, amount)
	}

	// 4. Insert wallet_transaction.
	txID := uuid.New()
	balanceBefore := balance
	balanceAfter := balance // balance unchanged by reserve (only frozen changes)
	const insertTx = `
		INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount,
			balance_before, balance_after, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	_, err = tx.Exec(ctx, insertTx,
		txID, walletID, idempotencyKey, "reserve", amount, balanceBefore, balanceAfter,
	)
	if err != nil {
		return nil, fmt.Errorf("wallet reserve insert tx: %w", err)
	}

	// 5. Update wallet: freeze amount + increment version.
	const updateWallet = `
		UPDATE wallets SET frozen = frozen + $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND version = $3
	`
	tag, err := tx.Exec(ctx, updateWallet, amount, walletID, version)
	if err != nil {
		return nil, fmt.Errorf("wallet reserve update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("wallet reserve: optimistic lock conflict (version=%d)", version)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wallet reserve commit: %w", err)
	}

	return &domain.WalletTransaction{
		ID:             txID,
		WalletID:       walletID,
		IdempotencyKey: idempotencyKey,
		TxType:         domain.WalletTxReserve,
		Amount:         amount,
		BalanceBefore:  balanceBefore,
		BalanceAfter:   balanceAfter,
	}, nil
}

// TopUp credits an amount to the wallet balance atomically using optimistic
// locking, recording a 'topup' wallet_transaction. Returns the existing
// transaction if the idempotency key was already used.
func (r *PostgresRepository) TopUp(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wallet topup begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("wallet topup: amount must be positive")
	}

	// 1. Check for existing idempotent transaction (scoped to this wallet so
	// identical keys from different users can never resolve to the same tx).
	existing, err := findIdempotentTx(ctx, tx, walletID, idempotencyKey)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("wallet topup commit: %w", cerr)
		}
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("wallet topup idempotent check: %w", err)
	}

	// 2. Lock the wallet row for update.
	var balance decimal.Decimal
	var balanceStr string
	var version int64
	const lockQuery = `
		SELECT balance, version FROM wallets WHERE id = $1 FOR UPDATE
	`
	lockRow := tx.QueryRow(ctx, lockQuery, walletID)
	if err := lockRow.Scan(&balanceStr, &version); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("wallet topup: wallet not found: %w", err)
		}
		return nil, fmt.Errorf("wallet topup lock: %w", err)
	}
	balance = parseDecimalStr(balanceStr)

	balanceBefore := balance
	balanceAfter := balance.Add(amount)

	// 3. Insert wallet_transaction.
	txID := uuid.New()
	const insertTx = `
		INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount,
			balance_before, balance_after, created_at)
		VALUES ($1, $2, $3, 'topup', $4, $5, $6, NOW())
	`
	_, err = tx.Exec(ctx, insertTx,
		txID, walletID, idempotencyKey, amount, balanceBefore, balanceAfter,
	)
	if err != nil {
		return nil, fmt.Errorf("wallet topup insert tx: %w", err)
	}

	// 4. Update wallet: balance += amount + increment version.
	const updateWallet = `
		UPDATE wallets SET balance = balance + $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND version = $3
	`
	tag, err := tx.Exec(ctx, updateWallet, amount, walletID, version)
	if err != nil {
		return nil, fmt.Errorf("wallet topup update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("wallet topup: optimistic lock conflict (version=%d)", version)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wallet topup commit: %w", err)
	}

	return &domain.WalletTransaction{
		ID:             txID,
		WalletID:       walletID,
		IdempotencyKey: idempotencyKey,
		TxType:         domain.WalletTxTopup,
		Amount:         amount,
		BalanceBefore:  balanceBefore,
		BalanceAfter:   balanceAfter,
	}, nil
}

// Transfer atomically moves an amount between two wallets in one transaction:
// a negative transfer_out on the source and a positive transfer_in on the
// destination, cross-referenced by wallet id. Replaying the same idempotency
// key against the same source wallet returns the original transfer_out without
// double-debiting. Returns ErrInsufficientBalance when the source lacks
// available balance and ErrNotFound when either wallet is missing.
func (r *PostgresRepository) Transfer(ctx context.Context, fromWalletID, toWalletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("wallet transfer: amount must be positive")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wallet transfer begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Idempotent replay: same (from_wallet, key) resolves to the original
	//    transfer_out so retries never double-debit. A key that maps to a
	//    *different* amount is a conflict, never a replay — returning the
	//    original tx would report success while moving no money.
	existing, err := findIdempotentTx(ctx, tx, fromWalletID, idempotencyKey)
	if err == nil {
		if !existing.Amount.Abs().Equal(amount) {
			return nil, fmt.Errorf("%w: key=%s stored=%s requested=%s",
				ErrIdempotencyMismatch, idempotencyKey, existing.Amount.Abs(), amount)
		}
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("wallet transfer commit: %w", cerr)
		}
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("wallet transfer idempotent check: %w", err)
	}

	// 2. Lock both wallets in canonical (sorted-by-id) order so a concurrent
	//    A→B and B→A transfer can never deadlock on each other's rows.
	type lockedWallet struct {
		balance decimal.Decimal
		frozen  decimal.Decimal
		version int64
	}
	lockOrder := [2]uuid.UUID{fromWalletID, toWalletID}
	if lockOrder[0].String() > lockOrder[1].String() {
		lockOrder[0], lockOrder[1] = lockOrder[1], lockOrder[0]
	}
	locked := make(map[uuid.UUID]lockedWallet, 2)
	for _, id := range lockOrder {
		var lw lockedWallet
		var balStr, frozenStr string
		if err := tx.QueryRow(ctx,
			`SELECT balance, frozen, version FROM wallets WHERE id = $1 FOR UPDATE`,
			id).Scan(&balStr, &frozenStr, &lw.version); err != nil {
			if err == pgx.ErrNoRows {
				label := "destination"
				if id == fromWalletID {
					label = "source"
				}
				return nil, fmt.Errorf("%w: %s wallet", ErrNotFound, label)
			}
			return nil, fmt.Errorf("wallet transfer lock %s: %w", id, err)
		}
		lw.balance = parseDecimalStr(balStr)
		lw.frozen = parseDecimalStr(frozenStr)
		locked[id] = lw
	}
	fromLock := locked[fromWalletID]
	toLock := locked[toWalletID]

	// 3. Validate available balance on the source.
	available := fromLock.balance.Sub(fromLock.frozen)
	if available.LessThan(amount) {
		return nil, fmt.Errorf("%w: available=%s required=%s", errInsufficientBalance, available, amount)
	}

	// 4. Write both ledger rows: transfer_out (source, -amount) and
	//    transfer_in (destination, +amount), cross-referenced and sharing the
	//    same idempotency key.
	transferID := uuid.New()
	outTxID := uuid.New()
	inTxID := uuid.New()
	meta, err := json.Marshal(map[string]string{
		"transfer_id":    transferID.String(),
		"from_wallet_id": fromWalletID.String(),
		"to_wallet_id":   toWalletID.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("wallet transfer marshal metadata: %w", err)
	}
	const insertTx = `
		INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount,
			balance_before, balance_after, reference_type, reference_id, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
	`
	if _, err := tx.Exec(ctx, insertTx,
		outTxID, fromWalletID, idempotencyKey, domain.WalletTxTransferOut,
		amount.Neg(), fromLock.balance, fromLock.balance.Sub(amount),
		"balance_transfer", toWalletID, json.RawMessage(meta),
	); err != nil {
		return nil, fmt.Errorf("wallet transfer insert out: %w", err)
	}
	if _, err := tx.Exec(ctx, insertTx,
		inTxID, toWalletID, idempotencyKey, domain.WalletTxTransferIn,
		amount, toLock.balance, toLock.balance.Add(amount),
		"balance_transfer", fromWalletID, json.RawMessage(meta),
	); err != nil {
		return nil, fmt.Errorf("wallet transfer insert in: %w", err)
	}

	// 5. Update both wallets with optimistic locking.
	const updateSource = `
		UPDATE wallets SET balance = balance - $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND version = $3
	`
	tag, err := tx.Exec(ctx, updateSource, amount, fromWalletID, fromLock.version)
	if err != nil {
		return nil, fmt.Errorf("wallet transfer update source: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("wallet transfer: source optimistic lock conflict (version=%d)", fromLock.version)
	}
	destTag, err := tx.Exec(ctx,
		`UPDATE wallets SET balance = balance + $1, version = version + 1, updated_at = NOW() WHERE id = $2 AND version = $3`,
		amount, toWalletID, toLock.version)
	if err != nil {
		return nil, fmt.Errorf("wallet transfer update destination: %w", err)
	}
	if destTag.RowsAffected() == 0 {
		return nil, fmt.Errorf("wallet transfer: destination optimistic lock conflict (version=%d)", toLock.version)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wallet transfer commit: %w", err)
	}

	return &domain.WalletTransaction{
		ID:             outTxID,
		WalletID:       fromWalletID,
		IdempotencyKey: idempotencyKey,
		TxType:         domain.WalletTxTransferOut,
		Amount:         amount.Neg(),
		BalanceBefore:  fromLock.balance,
		BalanceAfter:   fromLock.balance.Sub(amount),
		ReferenceType:  "balance_transfer",
		ReferenceID:    &toWalletID,
	}, nil
}

// Spend atomically deducts `amount` from the wallet and records a negative
// wallet_transaction (tx_type 'subscription'). Idempotent replay returns the
// original record; insufficient balance returns ErrInsufficientBalance.
func (r *PostgresRepository) Spend(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("wallet spend: amount must be positive")
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wallet spend begin: %w", err)
	}
	defer tx.Rollback(ctx)

	existing, err := findIdempotentTx(ctx, tx, walletID, idempotencyKey)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("wallet spend commit: %w", cerr)
		}
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("wallet spend idempotent check: %w", err)
	}

	var balanceStr string
	var version int64
	if err := tx.QueryRow(ctx,
		`SELECT balance, version FROM wallets WHERE id = $1 FOR UPDATE`, walletID,
	).Scan(&balanceStr, &version); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("wallet spend: wallet not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("wallet spend lock: %w", err)
	}
	balance := parseDecimalStr(balanceStr)
	if balance.LessThan(amount) {
		return nil, ErrInsufficientBalance
	}

	balanceBefore := balance
	balanceAfter := balance.Sub(amount)
	txID := uuid.New()
	if _, err := tx.Exec(ctx,
		`INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount,
			balance_before, balance_after, created_at)
		 VALUES ($1, $2, $3, 'subscription', $4, $5, $6, NOW())`,
		txID, walletID, idempotencyKey, amount.Neg(), balanceBefore, balanceAfter,
	); err != nil {
		return nil, fmt.Errorf("wallet spend insert tx: %w", err)
	}

	tag, err := tx.Exec(ctx,
		`UPDATE wallets SET balance = balance - $1, version = version + 1, updated_at = NOW()
		 WHERE id = $2 AND version = $3`,
		amount, walletID, version)
	if err != nil {
		return nil, fmt.Errorf("wallet spend update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("wallet spend: optimistic lock conflict (version=%d)", version)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wallet spend commit: %w", err)
	}
	return &domain.WalletTransaction{
		ID:             txID,
		WalletID:       walletID,
		IdempotencyKey: idempotencyKey,
		TxType:         domain.WalletTxSubscription,
		Amount:         amount.Neg(),
		BalanceBefore:  balanceBefore,
		BalanceAfter:   balanceAfter,
	}, nil
}

// Commit finalizes a reserved transaction: balance -= amount, frozen -= amount.
func (r *PostgresRepository) Commit(ctx context.Context, txID uuid.UUID) error {
	dbTx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wallet commit begin: %w", err)
	}
	defer dbTx.Rollback(ctx)

	// 1. Find the reserve transaction and its wallet.
	var walletID uuid.UUID
	var amount decimal.Decimal
	var amountStr, txType string
	const findTx = `
		SELECT wallet_id, amount, tx_type FROM wallet_transactions WHERE id = $1 FOR UPDATE
	`
	if err := dbTx.QueryRow(ctx, findTx, txID).Scan(&walletID, &amountStr, &txType); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("wallet commit: transaction not found: %w", err)
		}
		return fmt.Errorf("wallet commit find tx: %w", err)
	}
	amount = parseDecimalStr(amountStr)
	if txType != "reserve" {
		return fmt.Errorf("wallet commit: %w: type=%s", ErrTxNotReserved, txType)
	}

	// 2. Update wallet_transaction type to 'charge'.
	const updateTx = `UPDATE wallet_transactions SET tx_type = 'charge' WHERE id = $1`
	if _, err := dbTx.Exec(ctx, updateTx, txID); err != nil {
		return fmt.Errorf("wallet commit update tx type: %w", err)
	}

	// 3. Update wallet: balance -= amount, frozen -= amount.
	const updateWallet = `
		UPDATE wallets SET balance = balance - $1, frozen = frozen - $1, updated_at = NOW()
		WHERE id = $2
	`
	if _, err := dbTx.Exec(ctx, updateWallet, amount, walletID); err != nil {
		return fmt.Errorf("wallet commit update wallet: %w", err)
	}

	return dbTx.Commit(ctx)
}

// Settle finalizes a reserved transaction against the ACTUAL final cost.
//   - finalAmount > reserved: charges the extra against available balance
//   - finalAmount < reserved: refunds the excess to the balance
//   - frozen is always fully released
//
// Returns ErrInsufficientBalance when the wallet cannot cover a larger
// final cost; the caller may fall back to Commit (reserved amount only).
func (r *PostgresRepository) Settle(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error {
	if finalAmount.LessThan(decimal.Zero) {
		return fmt.Errorf("wallet settle: final amount must not be negative: %s", finalAmount)
	}

	dbTx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wallet settle begin: %w", err)
	}
	defer dbTx.Rollback(ctx)

	// 1. Find the reserve transaction and its wallet.
	var walletID uuid.UUID
	var reserved decimal.Decimal
	var amountStr, txType string
	const findTx = `
		SELECT wallet_id, amount, tx_type FROM wallet_transactions WHERE id = $1 FOR UPDATE
	`
	if err := dbTx.QueryRow(ctx, findTx, txID).Scan(&walletID, &amountStr, &txType); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("wallet settle: transaction not found: %w", err)
		}
		return fmt.Errorf("wallet settle find tx: %w", err)
	}
	reserved = parseDecimalStr(amountStr)
	if txType != "reserve" {
		return fmt.Errorf("wallet settle: %w: type=%s", ErrTxNotReserved, txType)
	}

	// 2. Lock the wallet row.
	var balance, frozen decimal.Decimal
	var balanceStr, frozenStr string
	const lockWallet = `
		SELECT balance, frozen FROM wallets WHERE id = $1 FOR UPDATE
	`
	if err := dbTx.QueryRow(ctx, lockWallet, walletID).Scan(&balanceStr, &frozenStr); err != nil {
		return fmt.Errorf("wallet settle lock wallet: %w", err)
	}
	balance = parseDecimalStr(balanceStr)
	frozen = parseDecimalStr(frozenStr)

	// 3. If the final cost exceeds the reserved amount, the extra must be
	//    covered by the currently available balance — never go negative.
	diff := finalAmount.Sub(reserved)
	if diff.IsPositive() {
		available := balance.Sub(frozen)
		if available.LessThan(diff) {
			return fmt.Errorf("%w: available=%s extra_required=%s", errInsufficientBalance, available, diff)
		}
	}
	// The reserved amount was already frozen; the final balance nets exactly
	// -finalAmount (the frozen part is released, the extra/refund adjusts).
	balance = balance.Sub(finalAmount)
	frozen = frozen.Sub(reserved)

	// 4. Update wallet.
	if _, err := dbTx.Exec(ctx,
		`UPDATE wallets SET balance = $1, frozen = $2, version = version + 1, updated_at = NOW() WHERE id = $3`,
		balance.String(), frozen.String(), walletID); err != nil {
		return fmt.Errorf("wallet settle update wallet: %w", err)
	}

	// 5. Mark the transaction as a charge with the final amount.
	if _, err := dbTx.Exec(ctx,
		`UPDATE wallet_transactions SET tx_type = 'charge', amount = $1, balance_after = $2 WHERE id = $3`,
		finalAmount.String(), balance.String(), txID); err != nil {
		return fmt.Errorf("wallet settle update tx: %w", err)
	}

	return dbTx.Commit(ctx)
}

// Release cancels a reserved transaction: frozen -= amount.
func (r *PostgresRepository) Release(ctx context.Context, txID uuid.UUID) error {
	dbTx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wallet release begin: %w", err)
	}
	defer dbTx.Rollback(ctx)

	// 1. Find the reserve transaction.
	var walletID uuid.UUID
	var amount decimal.Decimal
	var amountStr, txType string
	const findTx = `
		SELECT wallet_id, amount, tx_type FROM wallet_transactions WHERE id = $1 FOR UPDATE
	`
	if err := dbTx.QueryRow(ctx, findTx, txID).Scan(&walletID, &amountStr, &txType); err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("wallet release: transaction not found: %w", err)
		}
		return fmt.Errorf("wallet release find tx: %w", err)
	}
	amount = parseDecimalStr(amountStr)
	if txType != "reserve" {
		return fmt.Errorf("wallet release: %w: type=%s", ErrTxNotReserved, txType)
	}

	// 2. Update wallet_transaction type to 'release'.
	const updateTx = `UPDATE wallet_transactions SET tx_type = 'release' WHERE id = $1`
	if _, err := dbTx.Exec(ctx, updateTx, txID); err != nil {
		return fmt.Errorf("wallet release update tx type: %w", err)
	}

	// 3. Update wallet: frozen -= amount.
	const updateWallet = `
		UPDATE wallets SET frozen = frozen - $1, updated_at = NOW()
		WHERE id = $2
	`
	if _, err := dbTx.Exec(ctx, updateWallet, amount, walletID); err != nil {
		return fmt.Errorf("wallet release update wallet: %w", err)
	}

	return dbTx.Commit(ctx)
}

// ListTransactions returns wallet transactions with pagination.
func (r *PostgresRepository) ListTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]domain.WalletTransaction, error) {
	if limit <= 0 {
		limit = 20
	}
	const query = `
		SELECT id, wallet_id, idempotency_key, tx_type, amount,
			balance_before, balance_after, reference_type, reference_id,
			metadata, created_at
		FROM wallet_transactions
		WHERE wallet_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("wallet list transactions: %w", err)
	}
	defer rows.Close()

	var txs []domain.WalletTransaction
	for rows.Next() {
		var tx domain.WalletTransaction
		var amountStr, balanceBeforeStr, balanceAfterStr string
		var refType, refID *string
		err := rows.Scan(&tx.ID, &tx.WalletID, &tx.IdempotencyKey, &tx.TxType,
			&amountStr, &balanceBeforeStr, &balanceAfterStr,
			&refType, &refID, &tx.Metadata, &tx.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("wallet list scan: %w", err)
		}
		tx.Amount = parseDecimalStr(amountStr)
		tx.BalanceBefore = parseDecimalStr(balanceBeforeStr)
		tx.BalanceAfter = parseDecimalStr(balanceAfterStr)
		if refType != nil {
			tx.ReferenceType = *refType
		}
		if refID != nil {
			rid, _ := uuid.Parse(*refID)
			tx.ReferenceID = &rid
		}
		txs = append(txs, tx)
	}
	return txs, rows.Err()
}

// --- helpers ---

func scanWallet(row pgx.Row) (*domain.Wallet, error) {
	var w domain.Wallet
	var balanceStr, frozenStr string
	err := row.Scan(&w.ID, &w.UserID, &w.TenantID,
		&balanceStr, &frozenStr, &w.Currency, &w.Version,
		&w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("wallet scan: %w", err)
	}
	w.Balance = parseDecimalStr(balanceStr)
	w.Frozen = parseDecimalStr(frozenStr)
	return &w, nil
}

// findIdempotentTx checks if a transaction with the given idempotency key
// already exists for the given wallet.
func findIdempotentTx(ctx context.Context, tx pgx.Tx, walletID uuid.UUID, key string) (*domain.WalletTransaction, error) {
	const query = `
		SELECT id, wallet_id, idempotency_key, tx_type, amount,
			balance_before, balance_after, created_at
		FROM wallet_transactions
		WHERE idempotency_key = $1 AND wallet_id = $2
	`
	row := tx.QueryRow(ctx, query, key, walletID)
	var wt domain.WalletTransaction
	var amountStr, balanceBeforeStr, balanceAfterStr string
	err := row.Scan(&wt.ID, &wt.WalletID, &wt.IdempotencyKey, &wt.TxType,
		&amountStr, &balanceBeforeStr, &balanceAfterStr, &wt.CreatedAt)
	if err != nil {
		return nil, err
	}
	wt.Amount = parseDecimalStr(amountStr)
	wt.BalanceBefore = parseDecimalStr(balanceBeforeStr)
	wt.BalanceAfter = parseDecimalStr(balanceAfterStr)
	return &wt, nil
}

func parseDecimalStr(v string) decimal.Decimal {
	d, err := decimal.NewFromString(v)
	if err != nil {
		return decimal.Zero
	}
	return d
}

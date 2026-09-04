package paymentorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository with PostgreSQL via pgx/v5.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

// ErrNotFound is returned when a payment order row does not exist.
var ErrNotFound = errors.New("paymentorder: not found")

const orderCols = `id, order_no, user_id, amount, currency, purpose, plan_id, channel, pay_method, status,
	gateway_trade_no, pay_url, notify_raw, paid_at, expires_at, created_at, updated_at,
	query_attempts, last_query_at, next_retry_at, review_reason`

func (r *PostgresRepository) Create(ctx context.Context, o *Order) error {
	if o.Purpose == "" {
		o.Purpose = "topup"
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO payment_orders
			(order_no, user_id, amount, currency, purpose, plan_id, channel, pay_method, status, expires_at, pay_url)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		o.OrderNo, o.UserID, o.Amount, o.Currency, o.Purpose, o.PlanID, o.Channel, o.PayMethod, o.Status, o.ExpiresAt, o.PayURL)
	if err != nil {
		return fmt.Errorf("paymentorder create: %w", err)
	}
	return nil
}

func scanOrder(row pgx.Row) (*Order, error) {
	var o Order
	var gatewayTradeNo, payURL *string
	var planID *uuid.UUID
	var notifyRaw []byte
	var paidAt *time.Time
	// Provider metadata (TH-P1-05): nullable columns scan into pointers so
	// pre-migration rows surface as nil, never fabricated zero values.
	var queryAttempts *int
	var lastQueryAt, nextRetryAt *time.Time
	var reviewReason *string
	err := row.Scan(&o.ID, &o.OrderNo, &o.UserID, &o.Amount, &o.Currency,
		&o.Purpose, &planID, &o.Channel, &o.PayMethod, &o.Status, &gatewayTradeNo, &payURL, &notifyRaw, &paidAt,
		&o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt,
		&queryAttempts, &lastQueryAt, &nextRetryAt, &reviewReason)
	if err != nil {
		return nil, err
	}
	o.GatewayTradeNo = gatewayTradeNo
	o.PayURL = payURL
	o.NotifyRaw = notifyRaw
	o.PaidAt = paidAt
	o.PlanID = planID
	o.QueryAttempts = queryAttempts
	o.LastQueryAt = lastQueryAt
	o.NextRetryAt = nextRetryAt
	o.ReviewReason = reviewReason
	return &o, nil
}

func (r *PostgresRepository) FindByOrderNo(ctx context.Context, orderNo string) (*Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM payment_orders WHERE order_no=$1`, orderNo))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("paymentorder find by order no: %w", err)
	}
	return o, nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id uuid.UUID) (*Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx, `SELECT `+orderCols+` FROM payment_orders WHERE id=$1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("paymentorder find by id: %w", err)
	}
	return o, nil
}

func (r *PostgresRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Order, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+orderCols+` FROM payment_orders WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("paymentorder list by user: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

func (r *PostgresRepository) List(ctx context.Context, limit, offset int, status *string, userID *uuid.UUID) ([]Order, error) {
	query := `SELECT ` + orderCols + ` FROM payment_orders`
	args := []any{}
	var clauses []string
	if status != nil {
		args = append(args, *status)
		clauses = append(clauses, fmt.Sprintf("status=$%d", len(args)))
	}
	if userID != nil {
		args = append(args, *userID)
		clauses = append(clauses, fmt.Sprintf("user_id=$%d", len(args)))
	}
	if len(clauses) > 0 {
		query += " WHERE " + joinClauses(clauses)
	}
	args = append(args, limit, offset)
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("paymentorder list: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

func joinClauses(clauses []string) string {
	out := ""
	for i, c := range clauses {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}

func scanOrders(rows pgx.Rows) ([]Order, error) {
	var out []Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresRepository) MarkPaid(ctx context.Context, id uuid.UUID, gatewayTradeNo string, notifyRaw []byte) (bool, error) {
	var raw any
	if len(notifyRaw) > 0 {
		if json.Valid(notifyRaw) {
			raw = json.RawMessage(notifyRaw)
		} else {
			raw = json.RawMessage(`{}`)
		}
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE payment_orders SET status='paid', gateway_trade_no=$2, notify_raw=$3, paid_at=NOW(), updated_at=NOW()
		 WHERE id=$1 AND status='pending'`,
		id, gatewayTradeNo, raw)
	if err != nil {
		return false, fmt.Errorf("paymentorder mark paid: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresRepository) MarkClosed(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE payment_orders SET status='closed', updated_at=NOW() WHERE id=$1 AND status='pending'`, id)
	if err != nil {
		return fmt.Errorf("paymentorder mark closed: %w", err)
	}
	return nil
}

// RecordProviderQuery implements Repository. The UPDATE touches only the
// TH-P1-05 metadata columns; status, amount and payment fields are never
// modified, so a retry bookkeeping bug can never settle an order.
func (r *PostgresRepository) RecordProviderQuery(ctx context.Context, id uuid.UUID, nextRetryAt *time.Time) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE payment_orders
		 SET query_attempts = COALESCE(query_attempts, 0) + 1,
		     last_query_at = NOW(),
		     next_retry_at = $2,
		     updated_at = NOW()
		 WHERE id=$1`,
		id, nextRetryAt)
	if err != nil {
		return fmt.Errorf("paymentorder record provider query: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetReviewReason implements Repository. Empty reason stores NULL (clears
// the flag); status and amount are never modified.
func (r *PostgresRepository) SetReviewReason(ctx context.Context, id uuid.UUID, reason string) error {
	var value any
	if reason != "" {
		value = reason
	}
	tag, err := r.pool.Exec(ctx,
		`UPDATE payment_orders SET review_reason=$2, updated_at=NOW() WHERE id=$1`,
		id, value)
	if err != nil {
		return fmt.Errorf("paymentorder set review reason: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPendingCandidates implements Repository (TH-P1-CW-01). It is the
// scanner's candidate query: pending orders at least as old as olderThan,
// optionally restricted to one channel, in a deterministic oldest-first
// order with order_no as the tiebreak, capped at limit. It is a pure
// SELECT — expiry and retry eligibility are enforced by the worker-level
// rules (internal/worker/paymentscan), and nothing here ever writes.
func (r *PostgresRepository) ListPendingCandidates(ctx context.Context, olderThan time.Time, limit int, channel *string) ([]Order, error) {
	query := `SELECT ` + orderCols + ` FROM payment_orders WHERE status=$1 AND created_at<=$2`
	args := []any{StatusPending, olderThan}
	if channel != nil {
		args = append(args, *channel)
		query += fmt.Sprintf(" AND channel=$%d", len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY created_at ASC, order_no ASC LIMIT $%d", len(args))

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("paymentorder list pending candidates: %w", err)
	}
	defer rows.Close()
	return scanOrders(rows)
}

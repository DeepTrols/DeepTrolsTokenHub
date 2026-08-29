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
	gateway_trade_no, pay_url, notify_raw, paid_at, expires_at, created_at, updated_at`

func (r *PostgresRepository) Create(ctx context.Context, o *Order) error {
	if o.Purpose == "" {
		o.Purpose = "topup"
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO payment_orders
			(order_no, user_id, amount, currency, purpose, plan_id, channel, pay_method, status, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		o.OrderNo, o.UserID, o.Amount, o.Currency, o.Purpose, o.PlanID, o.Channel, o.PayMethod, o.Status, o.ExpiresAt)
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
	err := row.Scan(&o.ID, &o.OrderNo, &o.UserID, &o.Amount, &o.Currency,
		&o.Purpose, &planID, &o.Channel, &o.PayMethod, &o.Status, &gatewayTradeNo, &payURL, &notifyRaw, &paidAt,
		&o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	o.GatewayTradeNo = gatewayTradeNo
	o.PayURL = payURL
	o.NotifyRaw = notifyRaw
	o.PaidAt = paidAt
	o.PlanID = planID
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

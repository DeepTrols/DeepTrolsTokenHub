package budget

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

func (r *PostgresRepository) ListByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.Budget, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, period, limit_amount, spent_amount, status, created_at, updated_at
		 FROM budgets WHERE tenant_id = $1 ORDER BY created_at ASC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	budgets := make([]domain.Budget, 0)
	for rows.Next() {
		var b domain.Budget
		var limitStr, spentStr string
		if err := rows.Scan(&b.ID, &b.TenantID, &b.Period, &limitStr, &spentStr,
			&b.Status, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		b.LimitAmount = mustDecimal(limitStr)
		b.SpentAmount = mustDecimal(spentStr)
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}

func (r *PostgresRepository) ListRequestsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.BudgetRequest, error) {
	return r.listRequests(ctx, `WHERE tenant_id = $1 ORDER BY created_at DESC`, tenantID)
}

func (r *PostgresRepository) ListRequests(ctx context.Context) ([]domain.BudgetRequest, error) {
	return r.listRequests(ctx, `ORDER BY created_at DESC`)
}

func (r *PostgresRepository) listRequests(ctx context.Context, where string, args ...any) ([]domain.BudgetRequest, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, requested_amount, reason, status, reviewer_id, reviewed_at, created_at
		 FROM budget_requests `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := make([]domain.BudgetRequest, 0)
	for rows.Next() {
		var req domain.BudgetRequest
		var amountStr string
		if err := rows.Scan(&req.ID, &req.TenantID, &amountStr, &req.Reason,
			&req.Status, &req.ReviewerID, &req.ReviewedAt, &req.CreatedAt); err != nil {
			return nil, err
		}
		req.RequestedAmount = mustDecimal(amountStr)
		requests = append(requests, req)
	}
	return requests, rows.Err()
}

func (r *PostgresRepository) CreateRequest(ctx context.Context, req *domain.BudgetRequest) error {
	if req.ID == uuid.Nil {
		req.ID = uuid.New()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO budget_requests (id, tenant_id, requested_amount, reason, status, created_at)
		 VALUES ($1, $2, $3, $4, 'pending', $5)`,
		req.ID, req.TenantID, req.RequestedAmount, req.Reason, req.CreatedAt)
	return err
}

func (r *PostgresRepository) ApproveRequest(ctx context.Context, requestID, reviewerID uuid.UUID) (*domain.Budget, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var tenantID uuid.UUID
	var amountStr string
	if err := tx.QueryRow(ctx,
		`SELECT tenant_id, requested_amount::text FROM budget_requests WHERE id = $1 FOR UPDATE`,
		requestID).Scan(&tenantID, &amountStr); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("budget request %s not found", requestID)
		}
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE budget_requests SET status = 'approved', reviewer_id = $1, reviewed_at = NOW()
		 WHERE id = $2`, reviewerID, requestID); err != nil {
		return nil, err
	}

	var budgetID uuid.UUID
	var limitStr string
	err = tx.QueryRow(ctx,
		`INSERT INTO budgets (tenant_id, period, limit_amount, status)
		 VALUES ($1, 'monthly', $2, 'active')
		 ON CONFLICT (tenant_id, period) DO UPDATE SET
		   limit_amount = budgets.limit_amount + EXCLUDED.limit_amount,
		   updated_at = NOW()
		 RETURNING id, limit_amount::text`,
		tenantID, amountStr).Scan(&budgetID, &limitStr)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &domain.Budget{ID: budgetID, TenantID: tenantID, Period: domain.BudgetPeriodMonthly,
		LimitAmount: mustDecimal(limitStr), Status: domain.BudgetStatusActive}, nil
}

func (r *PostgresRepository) RejectRequest(ctx context.Context, requestID, reviewerID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE budget_requests SET status = 'rejected', reviewer_id = $1, reviewed_at = NOW()
		 WHERE id = $2`, reviewerID, requestID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("budget request %s not found", requestID)
	}
	return nil
}

func mustDecimal(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Zero
	}
	return d
}

// Package subscription activates subscription plans and tracks per-period free
// token quotas for users (shared by the console purchase flow, the payment
// notify path and the gateway billing allowance).
package subscription

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// ErrPlanNotFound is returned when a plan is missing or disabled.
var ErrPlanNotFound = errors.New("subscription: plan not found")

// Service activates plans and tracks quota against the shared pool.
type Service struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Plan is an enabled plan's identity fields.
type Plan struct {
	ID           string
	Name         string
	Price        decimal.Decimal
	DurationDays int
	TokenQuota   int64
}

func (s *Service) FindEnabled(ctx context.Context, planID uuid.UUID) (*Plan, error) {
	var p Plan
	var id uuid.UUID
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, price, duration_days, token_quota FROM subscription_plans
		 WHERE id = $1 AND enabled = TRUE`, planID).
		Scan(&id, &p.Name, &p.Price, &p.DurationDays, &p.TokenQuota)
	if err != nil {
		return nil, ErrPlanNotFound
	}
	p.ID = id.String()
	return &p, nil
}

// Activate creates a new subscription, stacking onto an existing active one
// (extending from the later of now / current expiry). Returns the new expiry.
// Quota plans start a 30-day free-token period.
func (s *Service) Activate(ctx context.Context, userID, planID uuid.UUID) (time.Time, error) {
	return s.ActivateWithAutoRenew(ctx, userID, planID, false)
}

// ActivateWithAutoRenew creates a new subscription (stacking onto an existing
// active one) and records the auto-renew consent.
func (s *Service) ActivateWithAutoRenew(ctx context.Context, userID, planID uuid.UUID, autoRenew bool) (time.Time, error) {
	plan, err := s.FindEnabled(ctx, planID)
	if err != nil {
		return time.Time{}, err
	}
	var base time.Time
	_ = s.pool.QueryRow(ctx,
		`SELECT expires_at FROM user_subscriptions
		 WHERE user_id = $1 AND status = 'active' AND expires_at > NOW()
		 ORDER BY expires_at DESC LIMIT 1`,
		userID).Scan(&base)
	now := time.Now().UTC()
	if base.Before(now) {
		base = now
	}
	expiresAt := base.AddDate(0, 0, plan.DurationDays)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO user_subscriptions (id, user_id, plan_id, plan_name, price, starts_at, expires_at, status, auto_renew, quota_reset_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', $8,
		         CASE WHEN $9 > 0 THEN NOW() + INTERVAL '30 days' ELSE NULL END)`,
		uuid.New(), userID, planID, plan.Name, plan.Price, base, expiresAt, autoRenew, plan.TokenQuota)
	if err != nil {
		return time.Time{}, err
	}
	return expiresAt, nil
}

// SetAutoRenew flips auto-renew consent for all of the user's active
// subscriptions (the user opted in at purchase or from the console).
func (s *Service) SetAutoRenew(ctx context.Context, userID uuid.UUID, autoRenew bool) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE user_subscriptions SET auto_renew = $2, updated_at = NOW()
		 WHERE user_id = $1 AND status = 'active' AND expires_at > NOW()`,
		userID, autoRenew)
	return err
}

// RemainingQuota returns the free-token allowance left on the user's active
// quota plan: (remaining, hasQuotaPlan, error). A missing/exhausted plan or a
// lapsed period yields hasQuotaPlan=true with the full quota so callers can
// offer the free allowance again after the reset.
func (s *Service) RemainingQuota(ctx context.Context, userID uuid.UUID) (int64, bool, error) {
	var quota, used int64
	var resetAt *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT p.token_quota, us.quota_used, us.quota_reset_at
		 FROM user_subscriptions us
		 JOIN subscription_plans p ON p.id = us.plan_id
		 WHERE us.user_id = $1 AND us.status = 'active' AND us.expires_at > NOW()
		   AND p.token_quota > 0
		 ORDER BY us.expires_at DESC LIMIT 1`,
		userID).Scan(&quota, &used, &resetAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if resetAt != nil && time.Now().After(*resetAt) {
		return quota, true, nil // period lapsed: full allowance returns
	}
	remaining := quota - used
	if remaining < 0 {
		remaining = 0
	}
	return remaining, true, nil
}

// ConsumeQuota atomically deducts tokens from the user's active quota plan,
// lazily resetting a lapsed period first. Returns the tokens actually consumed
// (0 when there is no quota plan, the allowance is exhausted, or a concurrent
// request won the race — callers then bill normally).
func (s *Service) ConsumeQuota(ctx context.Context, userID uuid.UUID, tokens int64) (int64, error) {
	if tokens <= 0 {
		return 0, nil
	}
	var subID uuid.UUID
	var quota int64
	err := s.pool.QueryRow(ctx,
		`SELECT us.id, p.token_quota
		 FROM user_subscriptions us
		 JOIN subscription_plans p ON p.id = us.plan_id
		 WHERE us.user_id = $1 AND us.status = 'active' AND us.expires_at > NOW()
		   AND p.token_quota > 0
		 ORDER BY us.expires_at DESC LIMIT 1`,
		userID).Scan(&subID, &quota)
	if err != nil {
		return 0, nil // no quota plan
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE user_subscriptions us
		 SET quota_used = CASE WHEN us.quota_reset_at IS NOT NULL AND us.quota_reset_at <= NOW()
		                       THEN $2 ELSE us.quota_used + $2 END,
		     quota_reset_at = CASE WHEN us.quota_reset_at IS NOT NULL AND us.quota_reset_at <= NOW()
		                           THEN NOW() + INTERVAL '30 days' ELSE us.quota_reset_at END,
		     updated_at = NOW()
		 WHERE us.id = $1
		   AND CASE WHEN us.quota_reset_at IS NOT NULL AND us.quota_reset_at <= NOW()
		            THEN $2 ELSE us.quota_used + $2 END <= $3`,
		subID, tokens, quota)
	if err != nil {
		return 0, nil // DB hiccup: bill normally rather than fail the request
	}
	if tag.RowsAffected() == 0 {
		return 0, nil // allowance exhausted or concurrent request won
	}
	return tokens, nil
}

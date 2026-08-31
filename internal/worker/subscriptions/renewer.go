package subscriptions

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/deeptrols/api/internal/repository/wallet"
	subscriptionsvc "github.com/deeptrols/api/internal/service/subscription"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// renewWindow is how early before expiry an auto-renew subscription is renewed.
const renewWindow = 24 * time.Hour

// Renewer auto-renews opted-in subscriptions from the user's wallet balance
// shortly before expiry.
type Renewer struct {
	pool    *pgxpool.Pool
	wallets wallet.Repository
	svc     *subscriptionsvc.Service
}

func NewRenewer(pool *pgxpool.Pool, wallets wallet.Repository, svc *subscriptionsvc.Service) *Renewer {
	return &Renewer{pool: pool, wallets: wallets, svc: svc}
}

type dueSubscription struct {
	subID, userID, planID uuid.UUID
	price                 decimal.Decimal
	expiresAt             time.Time
}

// Run renews all due auto-renew subscriptions and returns the count renewed.
// A user without sufficient balance is skipped (the subscription stays active
// until it lapses; the user can top up or renew manually).
func (r *Renewer) Run(ctx context.Context) (int, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT us.id, us.user_id, us.plan_id, p.price, us.expires_at
		 FROM user_subscriptions us
		 JOIN subscription_plans p ON p.id = us.plan_id
		 WHERE us.status = 'active' AND us.auto_renew = TRUE
		   AND us.expires_at > NOW() AND us.expires_at <= NOW() + $1`,
		renewWindow)
	if err != nil {
		return 0, err
	}
	var due []dueSubscription
	for rows.Next() {
		var d dueSubscription
		if err := rows.Scan(&d.subID, &d.userID, &d.planID, &d.price, &d.expiresAt); err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	renewed := 0
	for _, d := range due {
		wal, err := r.wallets.FindByUser(ctx, d.userID, nil)
		if err != nil || wal == nil {
			log.Printf("renewer: no wallet for user %s; skipping", d.userID)
			continue
		}
		if wal.Balance.LessThan(d.price) {
			log.Printf("renewer: insufficient balance for subscription %s; skipping", d.subID)
			continue
		}
		// Idempotent per subscription + period so replays never double-debit.
		idem := "subrenew:" + d.subID.String() + ":" + d.expiresAt.Format("2006-01-02")
		if _, err := r.wallets.Spend(ctx, wal.ID, d.price, idem); err != nil {
			if errors.Is(err, wallet.ErrInsufficientBalance) {
				log.Printf("renewer: insufficient balance for subscription %s", d.subID)
				continue
			}
			log.Printf("renewer: spend error for subscription %s: %v", d.subID, err)
			continue
		}
		if _, err := r.svc.ActivateWithAutoRenew(ctx, d.userID, d.planID, true); err != nil {
			log.Printf("renewer: activate error for subscription %s: %v", d.subID, err)
			continue
		}
		// The renewal created a new auto-renew subscription stacking from the
		// old expiry; supersede the original so it is never re-picked.
		if _, err := r.pool.Exec(ctx,
			`UPDATE user_subscriptions SET auto_renew = FALSE, updated_at = NOW() WHERE id = $1`,
			d.subID); err != nil {
			log.Printf("renewer: supersede error for subscription %s: %v", d.subID, err)
		}
		renewed++
	}
	return renewed, nil
}

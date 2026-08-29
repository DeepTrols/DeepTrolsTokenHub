// Package subscriptions runs maintenance tasks for the subscription lifecycle
// (new-api subscription parity): expiring subscriptions whose term has ended.
package subscriptions

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Expirer sweeps expired subscriptions into the 'expired' terminal state.
type Expirer struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Expirer {
	return &Expirer{pool: pool}
}

// Run marks every active subscription whose expiry has passed as expired and
// returns the number of rows flipped.
func (e *Expirer) Run(ctx context.Context) (int, error) {
	tag, err := e.pool.Exec(ctx,
		`UPDATE user_subscriptions SET status = 'expired', updated_at = NOW()
		 WHERE status = 'active' AND expires_at <= NOW()`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

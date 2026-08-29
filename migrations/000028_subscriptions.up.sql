-- 000028_subscriptions.up.sql
CREATE TABLE IF NOT EXISTS subscription_plans (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(128) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    price       DECIMAL(18,6) NOT NULL,
    duration_days INT NOT NULL CHECK (duration_days > 0),
    sort_order  INT NOT NULL DEFAULT 0,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_subscriptions (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id),
    plan_id    UUID NOT NULL REFERENCES subscription_plans(id),
    plan_name  VARCHAR(128) NOT NULL,
    price      DECIMAL(18,6) NOT NULL,
    starts_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    status     VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'expired', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_user_subscriptions_user ON user_subscriptions(user_id, created_at DESC);

-- Allow wallet_transactions to record subscription purchases.
ALTER TABLE wallet_transactions DROP CONSTRAINT IF EXISTS wallet_transactions_tx_type_check;
ALTER TABLE wallet_transactions ADD CONSTRAINT wallet_transactions_tx_type_check
    CHECK (tx_type IN ('topup', 'charge', 'refund', 'reserve', 'release',
           'transfer_in', 'transfer_out', 'compensate', 'subscription'));

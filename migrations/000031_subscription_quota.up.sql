-- 000031_subscription_quota.up.sql
-- Plans can carry a per-period free token allowance; usage within the
-- remaining allowance settles at zero (overflow bills the wallet normally).
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS token_quota BIGINT NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS quota_used BIGINT NOT NULL DEFAULT 0;
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS quota_reset_at TIMESTAMPTZ;

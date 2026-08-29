-- 000031_subscription_quota.down.sql
ALTER TABLE user_subscriptions DROP COLUMN IF EXISTS quota_reset_at;
ALTER TABLE user_subscriptions DROP COLUMN IF EXISTS quota_used;
ALTER TABLE subscription_plans DROP COLUMN IF EXISTS token_quota;

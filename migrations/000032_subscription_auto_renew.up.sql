-- 000032_subscription_auto_renew.up.sql
ALTER TABLE user_subscriptions ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN NOT NULL DEFAULT FALSE;

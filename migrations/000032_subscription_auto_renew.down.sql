-- 000032_subscription_auto_renew.down.sql
ALTER TABLE user_subscriptions DROP COLUMN IF EXISTS auto_renew;

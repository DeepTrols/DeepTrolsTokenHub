-- 000030_subscription_orders.down.sql
ALTER TABLE payment_orders DROP COLUMN IF EXISTS plan_id;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS purpose;

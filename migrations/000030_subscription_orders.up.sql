-- 000030_subscription_orders.up.sql
-- Payment orders can now purchase subscriptions (not only wallet topup).
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS purpose VARCHAR(16) NOT NULL DEFAULT 'topup'
    CHECK (purpose IN ('topup', 'subscription'));
ALTER TABLE payment_orders ADD COLUMN IF NOT EXISTS plan_id UUID REFERENCES subscription_plans(id);

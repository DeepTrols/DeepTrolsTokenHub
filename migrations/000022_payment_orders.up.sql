-- 000022_payment_orders.up.sql
-- Recharge/payment orders for the Alipay/WeChat gateway (epay first).
-- Crediting is NOT stored here: it reuses wallet_transactions with
-- idempotency_key = order_no (unique) to guarantee single credit.

CREATE TABLE IF NOT EXISTS payment_orders (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_no         VARCHAR(64) NOT NULL UNIQUE,
    user_id          UUID NOT NULL REFERENCES users(id),
    amount           DECIMAL(18,6) NOT NULL,
    currency         VARCHAR(8) NOT NULL DEFAULT 'CNY',
    channel          VARCHAR(32) NOT NULL,            -- epay / alipay / wechat
    pay_method       VARCHAR(32) NOT NULL,            -- alipay / wxpay
    status           VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending','paid','closed','refunded')),
    gateway_trade_no VARCHAR(128),
    pay_url          TEXT,
    notify_raw       JSONB,
    paid_at          TIMESTAMPTZ,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_orders_user ON payment_orders(user_id, created_at DESC);
CREATE INDEX idx_payment_orders_status ON payment_orders(status, expires_at);

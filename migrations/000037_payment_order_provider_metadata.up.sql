-- TH-P1-05: provider query/retry/review metadata for payment_orders.
--
-- Nullable additive columns only: pre-existing rows keep NULL values and
-- remain fully readable (no backfill, no NOT NULL risk). Semantics:
--
--   query_attempts  number of provider order-query attempts already made;
--                   NULL = never queried (legacy / brand-new order).
--   last_query_at   wall-clock time of the most recent provider query.
--   next_retry_at   when the compensation/query worker may retry this order;
--                   NULL = nothing scheduled. Powers query + compensation.
--   review_reason   why the order was flagged for manual review (e.g.
--                   amount_mismatch on a paid provider answer); NULL = not
--                   flagged. Powers reconciliation/review tracking.
--
-- Callback and reconciliation identity (order_no, gateway_trade_no, channel,
-- pay_method, amount) already exist on the table and are unchanged.

ALTER TABLE payment_orders ADD COLUMN query_attempts INTEGER;
ALTER TABLE payment_orders ADD COLUMN last_query_at TIMESTAMPTZ;
ALTER TABLE payment_orders ADD COLUMN next_retry_at TIMESTAMPTZ;
ALTER TABLE payment_orders ADD COLUMN review_reason TEXT;

-- Partial index for the compensation/query worker: only rows with a pending
-- retry schedule are indexed.
CREATE INDEX idx_payment_orders_next_retry
	ON payment_orders (next_retry_at)
	WHERE next_retry_at IS NOT NULL;

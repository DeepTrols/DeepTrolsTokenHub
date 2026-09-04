-- TH-P1-05 rollback: drop the nullable provider metadata columns.
-- Safe: the columns are purely additive operational metadata; dropping them
-- restores the pre-migration schema without touching order identity,
-- amounts, statuses, or callback payloads.

DROP INDEX IF EXISTS idx_payment_orders_next_retry;

ALTER TABLE payment_orders DROP COLUMN IF EXISTS review_reason;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS next_retry_at;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS last_query_at;
ALTER TABLE payment_orders DROP COLUMN IF EXISTS query_attempts;

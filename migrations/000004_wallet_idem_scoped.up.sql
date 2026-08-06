-- Make wallet idempotency keys scoped per wallet.
--
-- Previously wallet_transactions.idempotency_key had a GLOBAL unique
-- constraint while the reserve/topup lookup queried by key only. Two users
-- sending the same X-Request-ID would resolve to the SAME transaction, so the
-- second user could commit a charge against the first user's wallet.
--
-- Fix: scope the uniqueness to (wallet_id, idempotency_key) and clean up any
-- cross-wallet duplicates that the old global constraint allowed to be read
-- (the old constraint actually prevented duplicate INSERTs, but the lookup
-- still crossed wallet boundaries — this cleanup is defensive for data that
-- may have been created while the code bypassed the constraint).

-- Remove cross-wallet duplicates (keep the earliest row per idempotency_key).
DELETE FROM wallet_transactions a
USING wallet_transactions b
WHERE a.idempotency_key = b.idempotency_key
  AND a.created_at > b.created_at;

ALTER TABLE wallet_transactions DROP CONSTRAINT IF EXISTS wallet_transactions_idempotency_key_key;

CREATE UNIQUE INDEX idx_wallet_tx_idem_wallet
    ON wallet_transactions(wallet_id, idempotency_key);

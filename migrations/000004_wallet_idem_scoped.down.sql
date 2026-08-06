-- Revert: restore the global unique constraint on idempotency_key.
-- Remove cross-wallet duplicates first (the scoped index allowed the same key
-- on different wallets), keeping the earliest row per key.
DELETE FROM wallet_transactions a
USING wallet_transactions b
WHERE a.idempotency_key = b.idempotency_key
  AND a.created_at > b.created_at;

DROP INDEX IF EXISTS idx_wallet_tx_idem_wallet;

ALTER TABLE wallet_transactions
    ADD CONSTRAINT wallet_transactions_idempotency_key_key UNIQUE (idempotency_key);

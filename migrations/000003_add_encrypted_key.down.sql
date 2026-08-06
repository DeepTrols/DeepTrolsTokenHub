-- Remove the encrypted_key column added in 000003.
-- Only safe when no data relies on it; the code expects the column, so this
-- rollback is provided for completeness and should not normally be applied.
ALTER TABLE api_keys DROP COLUMN IF EXISTS encrypted_key;

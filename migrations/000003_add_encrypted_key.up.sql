-- Add the encrypted_key column to api_keys.
--
-- The API key repository (internal/repository/apikey/postgres.go) stores an
-- AES-encrypted (base64) copy of each API key plaintext here, but this column
-- was never added to the migration. The constraint/code drift caused all
-- apikey repository and handler tests to fail with "column encrypted_key of
-- relation api_keys does not exist". IF NOT EXISTS keeps this idempotent for
-- databases that already have the column (e.g. a dev DB created from an older
-- hand-run script).
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS encrypted_key TEXT;

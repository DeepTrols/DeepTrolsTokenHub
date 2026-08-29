-- 000021_system_settings.up.sql
-- Runtime-configurable platform settings (site & branding, payment, etc.).
-- Key/value table mirroring the new-api "Option" model: values are JSON.
-- A fixed set of keys plus defaults is enforced in service/setting.

CREATE TABLE IF NOT EXISTS system_settings (
    key        VARCHAR(128) PRIMARY KEY,
    value      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

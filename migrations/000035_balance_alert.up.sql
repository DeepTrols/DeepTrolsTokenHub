-- 000035_balance_alert.up.sql
-- Per-user low-balance alert threshold (CNY, decimal). 0 = disabled. The
-- wallet summary exposes below_threshold so the console can show a banner.
ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_alert_threshold NUMERIC(20,2) NOT NULL DEFAULT 0;

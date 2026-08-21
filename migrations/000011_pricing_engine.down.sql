DROP INDEX IF EXISTS idx_model_pricing_platform_dual;
DROP TABLE IF EXISTS pricing_settings;
ALTER TABLE model_pricing
    DROP COLUMN IF EXISTS price_type,
    DROP COLUMN IF EXISTS period;

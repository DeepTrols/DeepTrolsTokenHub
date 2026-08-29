-- 000034_pricing_tiers.down.sql
DROP INDEX IF EXISTS idx_model_pricing_platform_dual;
CREATE UNIQUE INDEX idx_model_pricing_platform_dual
    ON model_pricing(model_id, request_type, pricing_dimension, price_type, period)
    WHERE tenant_id IS NULL;

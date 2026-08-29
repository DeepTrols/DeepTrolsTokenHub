-- 000034_pricing_tiers.up.sql
-- Allow multiple platform pricing rows per (model, dim, type, period) when
-- they carry distinct tier conditions (NULL conditions coalesce to {} so the
-- single unconditional row per key is still enforced).
DROP INDEX IF EXISTS idx_model_pricing_platform_dual;
CREATE UNIQUE INDEX idx_model_pricing_platform_dual
    ON model_pricing(model_id, request_type, pricing_dimension, price_type, period,
                     COALESCE(conditions, '{}'::jsonb))
    WHERE tenant_id IS NULL;

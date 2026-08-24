-- 回退到 元/1K tokens 口径。
UPDATE model_pricing
SET unit_price    = unit_price / 1000,
    upstream_cost = upstream_cost / 1000,
    unit_name     = '1K tokens',
    updated_at    = NOW()
WHERE pricing_dimension IN ('input', 'output', 'cache_read', 'cache_write', 'reasoning');

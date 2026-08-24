-- 计费口径统一：token 维度价格单位从 元/1K tokens 改为 元/百万 tokens。
-- 历史行（含 000011 写入的 DeepSeek 成本行）数值 ×1000 即新口径原价，
-- 如 0.003 元/1K（= 3 元/百万）→ 3 元/百万，与官方价目页直接对齐。
UPDATE model_pricing
SET unit_price    = unit_price * 1000,
    upstream_cost = upstream_cost * 1000,
    unit_name     = '1M tokens',
    updated_at    = NOW()
WHERE pricing_dimension IN ('input', 'output', 'cache_read', 'cache_write', 'reasoning');

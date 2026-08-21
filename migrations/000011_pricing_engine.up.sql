-- B1 定价引擎：
-- 1. model_pricing 增加 价格类型（cost 成本 / sell 售价）与 时段（peak 高峰 / off_peak 非高峰）
-- 2. 以 DeepSeek 官方价目（2026-08-17 生效，元/百万 tokens）写入成本行（换算为 元/1K tokens），
--    并停用旧的占位售价行，售价 = 成本原价（无加价），在计费时实时计算

ALTER TABLE model_pricing
    ADD COLUMN price_type VARCHAR(16) NOT NULL DEFAULT 'sell'
        CHECK (price_type IN ('cost', 'sell')),
    ADD COLUMN period VARCHAR(16) NOT NULL DEFAULT 'off_peak'
        CHECK (period IN ('peak', 'off_peak'));

-- 平台级（tenant_id IS NULL）定价行支持 (模型, 类型, 时段) 唯一，便于幂等写入
CREATE UNIQUE INDEX idx_model_pricing_platform_dual
    ON model_pricing(model_id, request_type, pricing_dimension, price_type, period)
    WHERE tenant_id IS NULL;

-- DeepSeek 官方成本（2026-08-17 生效，元/百万 tokens → 元/1K tokens）
-- V4-Flash：off-peak cache 0.05 / input 1.5 / output 4.5；peak 0.10 / 3.0 / 9.0
-- V4-Pro：  off-peak cache 0.15 / input 4.5 / output 13.5；peak 0.30 / 9.0 / 27.0
INSERT INTO model_pricing
    (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency,
     upstream_cost, price_type, period, is_active, created_at, updated_at)
SELECT uuid_generate_v4(), m.id, 'chat', s.dimension, '1K tokens', s.price::numeric, 'CNY',
       s.price::numeric, 'cost', s.period, TRUE, NOW(), NOW()
FROM models m
JOIN (VALUES
    ('deepseek-v4-flash', 'cache_read', 'off_peak', '0.00005'),
    ('deepseek-v4-flash', 'cache_read', 'peak',     '0.0001'),
    ('deepseek-v4-flash', 'input',      'off_peak', '0.0015'),
    ('deepseek-v4-flash', 'input',      'peak',     '0.003'),
    ('deepseek-v4-flash', 'output',     'off_peak', '0.0045'),
    ('deepseek-v4-flash', 'output',     'peak',     '0.009'),
    ('deepseek-v4-pro',   'cache_read', 'off_peak', '0.00015'),
    ('deepseek-v4-pro',   'cache_read', 'peak',     '0.0003'),
    ('deepseek-v4-pro',   'input',      'off_peak', '0.0045'),
    ('deepseek-v4-pro',   'input',      'peak',     '0.009'),
    ('deepseek-v4-pro',   'output',     'off_peak', '0.0135'),
    ('deepseek-v4-pro',   'output',     'peak',     '0.027')
) AS s(code, dimension, period, price) ON m.code = s.code
ON CONFLICT (model_id, request_type, pricing_dimension, price_type, period)
    WHERE tenant_id IS NULL
    DO UPDATE SET unit_name = EXCLUDED.unit_name,
                  unit_price = EXCLUDED.unit_price,
                  upstream_cost = EXCLUDED.upstream_cost,
                  currency = EXCLUDED.currency,
                  is_active = TRUE,
                  updated_at = NOW();

-- 停用 deepseek 旧占位售价行（0.001 元/1K，输入输出同价），售价改由成本原价实时计算
UPDATE model_pricing SET is_active = FALSE, updated_at = NOW()
WHERE model_id IN (SELECT id FROM models WHERE code IN ('deepseek-v4-flash', 'deepseek-v4-pro'))
  AND price_type = 'sell'
  AND is_active = TRUE;

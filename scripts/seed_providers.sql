-- Seed DeepSeek and Qwen providers with models, channels, and instances.
-- Run: docker exec -i deeptrols-api-postgres-1 psql -U deeptrols -d deeptrols < scripts/seed_providers.sql
-- IMPORTANT: Replace REPLACE_WITH_* placeholders with real API keys before running.

-- 1. Models
INSERT INTO models (id, code, provider, category, display_name, description, context_window, max_output_tokens, status, release_stage, created_at, updated_at) VALUES
('a0000001-0001-0001-0001-000000000001', 'deepseek-v4-flash', 'deepseek', 'chat', 'DeepSeek V4 Flash', 'DeepSeek 快速模型', 131072, 32768, 'active', 'GA', NOW(), NOW()),
('a0000001-0001-0001-0001-000000000002', 'deepseek-v4-pro',   'deepseek', 'chat', 'DeepSeek V4 Pro',   'DeepSeek 旗舰推理模型', 131072, 65536, 'active', 'GA', NOW(), NOW()),
('a0000001-0001-0001-0001-000000000003', 'qwen3.5-plus',      'qwen',     'chat', 'Qwen 3.5 Plus',      '通义千问 3.5 Plus', 131072, 16384, 'active', 'GA', NOW(), NOW()),
('a0000001-0001-0001-0001-000000000004', 'qwen3.5-flash',     'qwen',     'chat', 'Qwen 3.5 Flash',     '通义千问 3.5 Flash 轻量', 131072, 8192, 'active', 'GA', NOW(), NOW()),
('a0000001-0001-0001-0001-000000000005', 'qwen3.7-plus',      'qwen',     'chat', 'Qwen 3.7 Plus',      '通义千问 3.7 Plus', 131072, 16384, 'active', 'GA', NOW(), NOW()),
('a0000001-0001-0001-0001-000000000006', 'qwen3.7-max',       'qwen',     'chat', 'Qwen 3.7 Max',       '通义千问 3.7 Max 旗舰', 131072, 65536, 'active', 'GA', NOW(), NOW())
ON CONFLICT (code) DO UPDATE SET display_name = EXCLUDED.display_name, updated_at = NOW();

-- 2. Model Pricing
INSERT INTO model_pricing (id, model_id, tenant_id, pricing_dimension, unit_name, unit_price, upstream_cost, currency, created_at, updated_at) VALUES
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000001', NULL, 'input',  '1K tokens', '0.002',  '0.002',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000001', NULL, 'output', '1K tokens', '0.006',  '0.006',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000002', NULL, 'input',  '1K tokens', '0.004',  '0.004',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000002', NULL, 'output', '1K tokens', '0.012',  '0.012',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000003', NULL, 'input',  '1K tokens', '0.001',  '0.001',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000003', NULL, 'output', '1K tokens', '0.003',  '0.003',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000004', NULL, 'input',  '1K tokens', '0.0005', '0.0005', 'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000004', NULL, 'output', '1K tokens', '0.0015', '0.0015', 'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000005', NULL, 'input',  '1K tokens', '0.001',  '0.001',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000005', NULL, 'output', '1K tokens', '0.003',  '0.003',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000006', NULL, 'input',  '1K tokens', '0.004',  '0.004',  'CNY', NOW(), NOW()),
(uuid_generate_v4(), 'a0000001-0001-0001-0001-000000000006', NULL, 'output', '1K tokens', '0.012',  '0.012',  'CNY', NOW(), NOW())
ON CONFLICT DO NOTHING;

-- 3. Channels
INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency, created_at, updated_at) VALUES
('b0000001-0001-0001-0001-000000000001', 'deepseek-flash', 'a0000001-0001-0001-0001-000000000001', 'shared', 100, 'healthy', 'active', 100, 10, NOW(), NOW()),
('b0000001-0001-0001-0001-000000000002', 'deepseek-pro',   'a0000001-0001-0001-0001-000000000002', 'shared', 100, 'healthy', 'active', 100, 10, NOW(), NOW()),
('b0000001-0001-0001-0001-000000000003', 'qwen35-plus',    'a0000001-0001-0001-0001-000000000003', 'shared', 100, 'healthy', 'active', 100, 10, NOW(), NOW()),
('b0000001-0001-0001-0001-000000000004', 'qwen35-flash',   'a0000001-0001-0001-0001-000000000004', 'shared', 100, 'healthy', 'active', 100, 10, NOW(), NOW()),
('b0000001-0001-0001-0001-000000000005', 'qwen37-plus',    'a0000001-0001-0001-0001-000000000005', 'shared', 100, 'healthy', 'active', 100, 10, NOW(), NOW()),
('b0000001-0001-0001-0001-000000000006', 'qwen37-max',     'a0000001-0001-0001-0001-000000000006', 'shared', 100, 'healthy', 'active', 100, 10, NOW(), NOW())
ON CONFLICT DO NOTHING;

-- 4. Channel Instances (API keys must be replaced with actual values before running)
INSERT INTO channel_instances (id, channel_id, instance_type, base_url, provider_route, current_load, max_load, config, status, created_at, updated_at) VALUES
-- DeepSeek
('c0000001-0001-0001-0001-000000000001', 'b0000001-0001-0001-0001-000000000001', 'serverless',
 'https://api.deepseek.com', 'deepseek-v4-flash', 0, 10,
 '{"api_key":"REPLACE_WITH_DEEPSEEK_API_KEY"}', 'active', NOW(), NOW()),
('c0000001-0001-0001-0001-000000000002', 'b0000001-0001-0001-0001-000000000002', 'serverless',
 'https://api.deepseek.com', 'deepseek-v4-pro', 0, 10,
 '{"api_key":"REPLACE_WITH_DEEPSEEK_API_KEY"}', 'active', NOW(), NOW()),
-- Qwen (Aliyun)
('c0000001-0001-0001-0001-000000000003', 'b0000001-0001-0001-0001-000000000003', 'serverless',
 'https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1', 'qwen3.5-plus', 0, 10,
 '{"api_key":"REPLACE_WITH_QWEN_API_KEY"}', 'active', NOW(), NOW()),
('c0000001-0001-0001-0001-000000000004', 'b0000001-0001-0001-0001-000000000004', 'serverless',
 'https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1', 'qwen3.5-flash', 0, 10,
 '{"api_key":"REPLACE_WITH_QWEN_API_KEY"}', 'active', NOW(), NOW()),
('c0000001-0001-0001-0001-000000000005', 'b0000001-0001-0001-0001-000000000005', 'serverless',
 'https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1', 'qwen3.7-plus', 0, 10,
 '{"api_key":"REPLACE_WITH_QWEN_API_KEY"}', 'active', NOW(), NOW()),
('c0000001-0001-0001-0001-000000000006', 'b0000001-0001-0001-0001-000000000006', 'serverless',
 'https://ws-m852wcwkjo52jqef.cn-beijing.maas.aliyuncs.com/compatible-mode/v1', 'qwen3.7-max', 0, 10,
 '{"api_key":"REPLACE_WITH_QWEN_API_KEY"}', 'active', NOW(), NOW())
ON CONFLICT DO NOTHING;

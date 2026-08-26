-- ============================================================================
-- Demo usage data seed — 让「账务管理」按账号展开后能看到真实的模型调用明细。
--
-- 只造数、不造币：只插入 usage_logs（以及演示所需 api_keys），不触碰钱包余额。
-- 幂等：先清掉本脚本此前生成的 demo-seed-* 行再重建，重复运行不会翻倍，
--       也不会影响真实的 playground-* 调用记录。
--
--   运行：docker exec -i deeptrolstokenhub-postgres-1 psql -U deeptrols -d deeptrols < scripts/seed_usage_demo.sql
--   注意：账号按邮箱匹配，脚本会对每个邮箱对应的用户生成一批调用记录。
-- ============================================================================

-- 1. 清除本脚本上次生成的演示数据（保证可重复执行）。
DELETE FROM usage_logs WHERE request_id LIKE 'demo-seed-%';

-- 2. 确保每个演示用户都有一条 api_key（usage_logs.api_key_id 非空）。
--    先清掉本脚本此前生成的演示密钥，再重建（ON CONFLICT DO NOTHING 无法把旧的
--    NULL 限额行刷新成新值）；确定性 key_hash 保证幂等。
--    演示密钥带真实的限制/模型白名单/最近使用时间，避免 NULL 触发扫描端到端回归
--    （前端「API 密钥」列表会把 NULL 限额渲染成 0）。
DELETE FROM api_keys WHERE key_hash LIKE 'demo-hash-%';
INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, status,
                      allowed_models, source_whitelist,
                      cumulative_limit, weekly_limit, monthly_limit,
                      over_limit_action, last_used_at, created_at, updated_at)
SELECT
  uuid_generate_v4(),
  u.id,
  'sk-',
  'demo-hash-' || u.id::text,
  'sk-****demo',
  CASE WHEN u.email = 'deeptrols@admin.com' THEN '演示密钥（管理员）' ELSE '演示密钥（测试）' END,
  'active',
  CASE WHEN u.email = 'deeptrols@admin.com'
       THEN ARRAY['deepseek-v4-flash', 'deepseek-v4-pro', 'qwen3.5-plus', 'qwen3.7-plus']
       ELSE ARRAY['deepseek-v4-flash', 'qwen3.5-flash'] END,
  '{}'::text[],
  1000.000000, 100.000000, 100.000000,
  'block',
  NOW() - interval '6 hours',
  NOW(), NOW()
FROM users u
WHERE u.email IN ('deeptrols@admin.com', 'deeptrols@test.com')
ON CONFLICT (key_hash) DO NOTHING;

-- 3. 批量生成 usage_logs。
--    每个 (账号, 模型) 桶生成 n 条记录，token 数随机、费用按挂牌价（与
--    scripts/seed_providers.sql 的定价一致）用 numeric 计算，绝不用 float。
WITH spec(user_email, model, n, in_price, out_price) AS (
  VALUES
    -- 管理员：深用 deepseek 旗舰 + qwen 中端，覆盖 4 个模型
    ('deeptrols@admin.com', 'deepseek-v4-flash', 6, 0.002, 0.006),
    ('deeptrols@admin.com', 'deepseek-v4-pro',   4, 0.004, 0.012),
    ('deeptrols@admin.com', 'qwen3.5-plus',      3, 0.001, 0.003),
    ('deeptrols@admin.com', 'qwen3.7-plus',      2, 0.001, 0.003),
    -- 测试账号：轻量使用，覆盖 2 个模型
    ('deeptrols@test.com',  'deepseek-v4-flash', 5, 0.002, 0.006),
    ('deeptrols@test.com',  'qwen3.5-flash',     3, 0.0005, 0.0015)
),
series AS (
  SELECT s.*, g.i,
         (50 + floor(random() * 1950)::int)  AS in_tok,   -- 50..1999
         (30 + floor(random() * 1470)::int)  AS out_tok   -- 30..1499
  FROM spec s
  CROSS JOIN LATERAL generate_series(1, s.n) AS g(i)
),
costed AS (
  SELECT s.*,
         round((s.in_tok::numeric * s.in_price + s.out_tok::numeric * s.out_price) / 1000, 6) AS cny
  FROM series s
)
INSERT INTO usage_logs
  (id, user_id, api_key_id, request_id, request_type,
   public_model_code, usage_source, usage_normalized, usage_raw,
   list_cost, discount_amount, final_cost, upstream_cost,
   currency, quota_deducted, wallet_charged, status, created_at)
SELECT
  uuid_generate_v4(),
  u.id,
  k.id,
  'demo-seed-' || u.email || '-' || s.model || '-' || s.i,
  'chat',
  s.model,
  'upstream',
  jsonb_build_object('input_tokens', s.in_tok, 'output_tokens', s.out_tok,
                     'reasoning_tokens', floor(s.out_tok * 0.3)::int,
                     'total_tokens', s.in_tok + s.out_tok),
  jsonb_build_object('prompt_tokens', s.in_tok, 'completion_tokens', s.out_tok,
                     'total_tokens', s.in_tok + s.out_tok),
  s.cny,   -- list_cost = 挂牌价
  0,       -- discount_amount
  s.cny,   -- final_cost = 实际扣费（演示取挂牌价）
  s.cny,   -- upstream_cost
  'CNY',
  s.in_tok + s.out_tok,
  s.cny,
  'completed',
  -- 时间向后铺开，模拟过去十余天的调用史
  NOW() - (s.i * 2 || ' days')::interval - (floor(random() * 12)::int || ' hours')::interval
FROM costed s
JOIN users u   ON u.email = s.user_email
JOIN api_keys k ON k.user_id = u.id AND k.key_hash = 'demo-hash-' || u.id::text;

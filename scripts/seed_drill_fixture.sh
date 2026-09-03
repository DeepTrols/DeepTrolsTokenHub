#!/usr/bin/env bash
# ============================================================
# DeepTrols 恢复演练数据播种（TH-P05-07 配套）
#
# 目的：在一个隔离的演练源库中构造"真实的钱与证据"数据，让
# restore_drill.sh 的内容级验证（钱包不变量 / 逐行校验和 /
# 分状态金额）在非空账本上被真正检验，而不是在空表上空转。
#
# 流程（全部真实执行）：
#   1. 目标防护：与 restore_drill.sh 同级 —— 生产形态名称一律拒绝。
#   2. DROP + CREATE 隔离播种库。
#   3. 按文件名顺序应用全部迁移（裸 SQL，与
#      internal/repository/testutil/schema.go 的 applyMigrations 同语义）。
#   4. 单事务播种资金/证据夹具：
#      - 3 用户 / 3 钱包 / 3 API Key
#      - 完整钱包生命周期：topup → reserve →（settle 低估退款 /
#        settle 超额补扣 / commit / release）→ 未完结 reserve 挂账
#      - subscription 消费、钱包间 transfer（out/in 配对）
#      - payment_orders 全 4 状态（paid 订单与 topup 的
#        idempotency_key=order_no 契约对齐）
#      - usage_logs（upstream / final_chunk / estimated 三种
#        usage_source 显式标记）+ charge_lines + provider_evidence
#      所有金额满足不变量：
#        W1 frozen == 未完结 reserve 之和（逐钱包）
#        W2 balance == 账本净额（charge=-amount，reserve/release=0，其余+amount）
#   5. 自校验：W1/W2 违例必须为 0，打印各表行数与钱包终态。
#
# 用法：
#   scripts/seed_drill_fixture.sh <SEED_DATABASE_URL>
#
# 退出码：0 成功；2 参数/防护拒绝；3 环境缺失；1 迁移/播种/自校验失败
#
# 安全约束：与 backup_db.sh 相同 —— URL 只解析进 libpq 环境变量。
# ============================================================

set -euo pipefail

usage() {
  echo "Usage: $0 <SEED_DATABASE_URL>" >&2
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

seed_url="$1"

command -v psql >/dev/null 2>&1 || { echo "seed: psql not found in PATH" >&2; exit 3; }

# ---- URL 解析（与 backup_db.sh 同规则） ----
urldecode() {
  local s="${1//+/ }"
  printf '%b' "${s//%/\\x}"
}

case "$seed_url" in
postgres://* | postgresql://*) ;;
*)
  echo "seed: DATABASE_URL must start with postgres:// or postgresql://" >&2
  exit 2
  ;;
esac

rest="${seed_url#*://}"
if [[ "$rest" != *"@"* ]]; then
  echo "seed: DATABASE_URL must embed credentials (user:pass@host)" >&2
  exit 2
fi
creds="${rest%%@*}"
hostdb="${rest#*@}"
hostdb="${hostdb%%\?*}"
if [[ "$creds" == *":"* ]]; then
  PU_USER="$(urldecode "${creds%%:*}")"
  PU_PASS="$(urldecode "${creds#*:}")"
else
  PU_USER="$(urldecode "$creds")"
  PU_PASS=""
fi
hostport="${hostdb%%/*}"
PU_DB="${hostdb#*/}"
PU_HOST="${hostport%%:*}"
if [[ "$hostport" == *":"* ]]; then
  PU_PORT="${hostport##*:}"
else
  PU_PORT=5432
fi
export PGHOST="$PU_HOST" PGPORT="$PU_PORT" PGUSER="$PU_USER" PGPASSWORD="$PU_PASS"

seed_db="$PU_DB"
seed_lc="$(printf '%s' "$seed_db" | tr '[:upper:]' '[:lower:]')"

# ---- 目标防护：本脚本会 DROP DATABASE，生产形态名称一律拒绝 ----
case "$seed_lc" in
deeptrols | prod | production | live | main)
  echo "seed: REFUSED — database '$seed_db' looks like a production database" >&2
  exit 2
  ;;
esac
if [[ ! "$seed_lc" =~ (drill|seed|test|staging|restore) ]]; then
  echo "seed: REFUSED — database '$seed_db' has no isolation marker" >&2
  echo "seed: name must contain one of: drill, seed, test, staging, restore" >&2
  exit 2
fi
if [[ ! "$seed_lc" =~ ^[a-z][a-z0-9_]{0,62}$ ]]; then
  echo "seed: REFUSED — database name contains unsafe characters: '$seed_db'" >&2
  exit 2
fi

seed_psql() {
  PGDATABASE="$seed_db" psql -X -q "$@"
}

redact() {
  local line
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ -n "$PU_PASS" ]]; then line="${line//"$PU_PASS"/[REDACTED]}"; fi
    printf '%s\n' "$line"
  done
}

run_redacted() {
  local out rc=0
  out="$("$@" 2>&1)" || rc=$?
  if [[ -n "$out" ]]; then
    printf '%s\n' "$out" | redact
  fi
  return "$rc"
}

echo "seed: start host=$PGHOST port=$PGPORT db=$seed_db user=$PGUSER"

# ---- 1. 全新播种库 ----
if ! run_redacted psql -X -q -d postgres -c "DROP DATABASE IF EXISTS \"$seed_db\" WITH (FORCE)"; then
  echo "seed: FAILED — could not drop existing seed database" >&2
  exit 1
fi
if ! run_redacted psql -X -q -d postgres -c "CREATE DATABASE \"$seed_db\""; then
  echo "seed: FAILED — could not create seed database" >&2
  exit 1
fi

# ---- 2. 迁移（文件名顺序裸 SQL，与测试基建同语义） ----
mig_dir="$(cd "$(dirname "$0")/../migrations" && pwd)"
mig_count=0
for f in "$mig_dir"/*.up.sql; do
  if ! run_redacted seed_psql -v ON_ERROR_STOP=1 -f "$f"; then
    echo "seed: FAILED — migration failed: $(basename "$f")" >&2
    exit 1
  fi
  mig_count=$((mig_count + 1))
done
echo "seed: applied $mig_count migrations"

# ---- 3. 播种（单事务；金额与钱包终态手工对账，见文件头注释） ----
# 终态对照：
#   alice: balance 94.000000  frozen 20.000000  version 10（1 笔未完结 reserve 20）
#   bob:   balance 135.000000 frozen 8.000000   version 4 （1 笔未完结 reserve 8）
#   carol: balance 45.000000  frozen 0          version 2
fixture_sql="$(mktemp)"
trap 'rm -f "$fixture_sql"' EXIT
cat >"$fixture_sql" <<'SQL'
BEGIN;

-- ---- users ----
INSERT INTO users (id, email, password_hash, display_name, status) VALUES
  ('a11ce000-0000-4000-8000-000000000001', 'alice@drill.example.com', 'argon2$drill-fixture-hash-a', 'Drill Alice', 'active'),
  ('b0b00000-0000-4000-8000-000000000002', 'bob@drill.example.com',   'argon2$drill-fixture-hash-b', 'Drill Bob',   'active'),
  ('ca401000-0000-4000-8000-000000000003', 'carol@drill.example.com', 'argon2$drill-fixture-hash-c', 'Drill Carol', 'active');

-- ---- wallets（终态见文件头） ----
INSERT INTO wallets (id, user_id, tenant_id, balance, frozen, currency, version) VALUES
  ('aa1ce000-0000-4000-8000-000000000001', 'a11ce000-0000-4000-8000-000000000001', NULL, 94.000000,  20.000000, 'CNY', 10),
  ('bb0b0000-0000-4000-8000-000000000002', 'b0b00000-0000-4000-8000-000000000002', NULL, 135.000000, 8.000000,  'CNY', 4),
  ('cc401000-0000-4000-8000-000000000003', 'ca401000-0000-4000-8000-000000000003', NULL, 45.000000,  0.000000,  'CNY', 2);

-- ---- api_keys ----
INSERT INTO api_keys (id, user_id, tenant_id, key_prefix, key_hash, masked_key, name, status) VALUES
  ('aaaa11ce-0000-4000-8000-000000000001', 'a11ce000-0000-4000-8000-000000000001', NULL, 'sk-drill-a', 'hash-drill-key-a', 'sk-drill-a****', 'drill-alice', 'active'),
  ('bbbb0b00-0000-4000-8000-000000000002', 'b0b00000-0000-4000-8000-000000000002', NULL, 'sk-drill-b', 'hash-drill-key-b', 'sk-drill-b****', 'drill-bob',   'active'),
  ('cccc4010-0000-4000-8000-000000000003', 'ca401000-0000-4000-8000-000000000003', NULL, 'sk-drill-c', 'hash-drill-key-c', 'sk-drill-c****', 'drill-carol', 'active');

-- ---- wallet_transactions：alice 完整生命周期 ----
-- topup 100（po1 已支付，idempotency_key=order_no 契约）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('a0000001-0000-4000-8000-000000000001', 'aa1ce000-0000-4000-8000-000000000001', 'DRILL-PO-1001', 'topup', 100.000000, 0.000000, 100.000000);
-- reserve 20：保持未完结（W1 挂账 20）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('a0000002-0000-4000-8000-000000000002', 'aa1ce000-0000-4000-8000-000000000001', 'drill-alice-reserve-1', 'reserve', 20.000000, 100.000000, 100.000000);
-- reserve 15 → settle 实际 12（低估退款；行被改写为 charge，amount=终值）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('a0000003-0000-4000-8000-000000000003', 'aa1ce000-0000-4000-8000-000000000001', 'drill-alice-reserve-2', 'charge', 12.000000, 100.000000, 88.000000);
-- reserve 30 → commit 30（行被改写为 charge）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('a0000004-0000-4000-8000-000000000004', 'aa1ce000-0000-4000-8000-000000000001', 'drill-alice-reserve-3', 'charge', 30.000000, 88.000000, 58.000000);
-- reserve 5 → release（余额不动，frozen 归还）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('a0000005-0000-4000-8000-000000000005', 'aa1ce000-0000-4000-8000-000000000001', 'drill-alice-reserve-4', 'release', 5.000000, 58.000000, 58.000000);
-- 手动 topup 50
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('a0000006-0000-4000-8000-000000000006', 'aa1ce000-0000-4000-8000-000000000001', 'drill-alice-manual-topup', 'topup', 50.000000, 58.000000, 108.000000);
-- reserve 10 → settle 实际 14（超额补扣 4 来自可用余额）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('a0000007-0000-4000-8000-000000000007', 'aa1ce000-0000-4000-8000-000000000001', 'drill-alice-reserve-5', 'charge', 14.000000, 108.000000, 94.000000);

-- ---- wallet_transactions：bob ----
-- topup 200（po3 已支付）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('b0000001-0000-4000-8000-000000000001', 'bb0b0000-0000-4000-8000-000000000002', 'DRILL-PO-1003', 'topup', 200.000000, 0.000000, 200.000000);
-- subscription 消费 25（amount 负值，与仓储 Spend 一致）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('b0000002-0000-4000-8000-000000000002', 'bb0b0000-0000-4000-8000-000000000002', 'drill-bob-sub-1', 'subscription', -25.000000, 200.000000, 175.000000);
-- transfer_out 40 → carol（amount 负值，交叉引用）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, reference_type, reference_id)
VALUES ('b0000003-0000-4000-8000-000000000003', 'bb0b0000-0000-4000-8000-000000000002', 'drill-transfer-1', 'transfer_out', -40.000000, 175.000000, 135.000000, 'balance_transfer', 'cc401000-0000-4000-8000-000000000003');
-- reserve 8：保持未完结（W1 挂账 8）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after)
VALUES ('b0000004-0000-4000-8000-000000000004', 'bb0b0000-0000-4000-8000-000000000002', 'drill-bob-reserve-1', 'reserve', 8.000000, 135.000000, 135.000000);

-- ---- wallet_transactions：carol ----
-- transfer_in 40（与 bob 共享 idempotency_key，唯一性按钱包作用域）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, reference_type, reference_id)
VALUES ('c0000001-0000-4000-8000-000000000001', 'cc401000-0000-4000-8000-000000000003', 'drill-transfer-1', 'transfer_in', 40.000000, 0.000000, 40.000000, 'balance_transfer', 'bb0b0000-0000-4000-8000-000000000002');
-- refund 5（po4 退款）
INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, reference_type)
VALUES ('c0000002-0000-4000-8000-000000000002', 'cc401000-0000-4000-8000-000000000003', 'drill-refund-po-1004', 'refund', 5.000000, 40.000000, 45.000000, 'payment_refund');

-- ---- payment_orders：4 种状态全覆盖 ----
INSERT INTO payment_orders (id, order_no, user_id, amount, currency, channel, pay_method, status, gateway_trade_no, pay_url, paid_at, expires_at) VALUES
  ('d0000001-0000-4000-8000-000000000001', 'DRILL-PO-1001', 'a11ce000-0000-4000-8000-000000000001', 100.000000, 'CNY', 'epay', 'alipay', 'paid',     'DRILL-TRADE-9001', NULL, '2026-09-01 10:00:00+00', '2026-09-01 10:30:00+00'),
  ('d0000002-0000-4000-8000-000000000002', 'DRILL-PO-1002', 'a11ce000-0000-4000-8000-000000000001', 50.000000,  'CNY', 'epay', 'wxpay',  'pending',  NULL,               'https://pay.drill.example.com/pay?order=DRILL-PO-1002', NULL, '2026-09-03 12:30:00+00'),
  ('d0000003-0000-4000-8000-000000000003', 'DRILL-PO-1003', 'b0b00000-0000-4000-8000-000000000002', 200.000000, 'CNY', 'epay', 'alipay', 'paid',     'DRILL-TRADE-9003', NULL, '2026-09-02 09:00:00+00', '2026-09-02 09:30:00+00'),
  ('d0000004-0000-4000-8000-000000000004', 'DRILL-PO-1004', 'ca401000-0000-4000-8000-000000000003', 5.000000,   'CNY', 'epay', 'wxpay',  'refunded', 'DRILL-TRADE-9004', NULL, '2026-09-02 15:00:00+00', '2026-09-02 15:30:00+00'),
  ('d0000005-0000-4000-8000-000000000005', 'DRILL-PO-1005', 'a11ce000-0000-4000-8000-000000000001', 50.000000,  'CNY', 'epay', 'alipay', 'closed',   NULL,               NULL, NULL, '2026-09-02 18:30:00+00');

-- ---- usage_logs：三种 usage_source 显式标记（不变量 #4） ----
INSERT INTO usage_logs (id, tenant_id, user_id, api_key_id, request_id, request_type, public_model_code, upstream_model_code,
                        usage_source, usage_raw, usage_normalized, estimated_cost, list_cost, discount_amount, final_cost,
                        currency, quota_deducted, wallet_charged, status) VALUES
  ('e0000001-0000-4000-8000-000000000001', NULL, 'a11ce000-0000-4000-8000-000000000001', 'aaaa11ce-0000-4000-8000-000000000001',
   'drill-req-001', 'chat.completion', 'gpt-4o', 'gpt-4o-2024-11-20',
   'upstream', '{"prompt_tokens":100000,"completion_tokens":500000}'::jsonb, '{"input_tokens":100000,"output_tokens":500000}'::jsonb,
   12.000000, 12.000000, 0.000000, 12.000000, 'CNY', 0, 12.000000, 'completed'),
  ('e0000002-0000-4000-8000-000000000002', NULL, 'a11ce000-0000-4000-8000-000000000001', 'aaaa11ce-0000-4000-8000-000000000001',
   'drill-req-002', 'chat.completion', 'gpt-4o', 'gpt-4o-2024-11-20',
   'final_chunk', '{"prompt_tokens":500000,"completion_tokens":1000000}'::jsonb, '{"input_tokens":500000,"output_tokens":1000000}'::jsonb,
   30.000000, 30.000000, 0.000000, 30.000000, 'CNY', 0, 30.000000, 'completed'),
  ('e0000003-0000-4000-8000-000000000003', NULL, 'b0b00000-0000-4000-8000-000000000002', 'bbbb0b00-0000-4000-8000-000000000002',
   'drill-req-003', 'embedding', 'text-embedding-3-large', 'text-embedding-3-large',
   'estimated', NULL, '{"input_tokens":25000}'::jsonb,
   0.500000, 0.500000, 0.000000, 0.000000, 'CNY', 0, 0.000000, 'failed');

UPDATE usage_logs SET error_code = 'upstream_429', error_message = 'drill fixture: upstream rate limited' WHERE id = 'e0000003-0000-4000-8000-000000000003';

-- ---- charge_lines：行合计与 usage_logs.final_cost 对齐 ----
INSERT INTO charge_lines (id, usage_log_id, dimension, unit_name, quantity, unit_price, line_cost, price_source, price_version) VALUES
  ('c1100001-0000-4000-8000-000000000001', 'e0000001-0000-4000-8000-000000000001', 'token', 'input_tokens',  100000, 0.0000200000, 2.000000,  'pricer', 1),
  ('c1100002-0000-4000-8000-000000000002', 'e0000001-0000-4000-8000-000000000001', 'token', 'output_tokens', 500000, 0.0000200000, 10.000000, 'pricer', 1),
  ('c1100003-0000-4000-8000-000000000003', 'e0000002-0000-4000-8000-000000000002', 'token', 'input_tokens',  500000, 0.0000200000, 10.000000, 'pricer', 1),
  ('c1100004-0000-4000-8000-000000000004', 'e0000002-0000-4000-8000-000000000002', 'token', 'output_tokens', 1000000, 0.0000200000, 20.000000, 'pricer', 1);

-- ---- provider_evidence：L1 证据（含成功与失败两态） ----
INSERT INTO provider_evidence (id, usage_log_id, provider, provider_request_id, status_code, duration_ms, usage_raw, provider_cost, provider_currency, error_message) VALUES
  ('f0000001-0000-4000-8000-000000000001', 'e0000001-0000-4000-8000-000000000001', 'openai', 'chatcmpl-drill-001', 200, 4211,
   '{"prompt_tokens":100000,"completion_tokens":500000,"total_tokens":600000}'::jsonb, 0.900000, 'USD', NULL),
  ('f0000002-0000-4000-8000-000000000002', 'e0000002-0000-4000-8000-000000000002', 'openai', 'chatcmpl-drill-002', 200, 9135,
   '{"prompt_tokens":500000,"completion_tokens":1000000,"total_tokens":1500000}'::jsonb, 2.250000, 'USD', NULL),
  ('f0000003-0000-4000-8000-000000000003', 'e0000003-0000-4000-8000-000000000003', 'openai', NULL, 429, 87,
   NULL, NULL, NULL, 'drill fixture: upstream rate limited');

COMMIT;
SQL

if ! run_redacted seed_psql -v ON_ERROR_STOP=1 -f "$fixture_sql"; then
  echo "seed: FAILED — fixture insert failed" >&2
  exit 1
fi
echo "seed: fixture inserted"

# ---- 4. 自校验：夹具自身必须满足钱包不变量 ----
w1="$(seed_psql -A -t -c "SELECT count(*) FROM (SELECT w.id FROM wallets w LEFT JOIN (SELECT wallet_id, COALESCE(sum(amount),0) AS open_reserve FROM wallet_transactions WHERE tx_type='reserve' GROUP BY wallet_id) r ON r.wallet_id = w.id WHERE w.frozen <> COALESCE(r.open_reserve,0)) x")"
w2="$(seed_psql -A -t -c "SELECT count(*) FROM (SELECT w.id FROM wallets w LEFT JOIN (SELECT wallet_id, sum(CASE WHEN tx_type IN ('reserve','release') THEN 0 WHEN tx_type='charge' THEN -amount ELSE amount END) AS s FROM wallet_transactions GROUP BY wallet_id) l ON l.wallet_id = w.id WHERE w.balance <> COALESCE(l.s,0)) x")"
echo "seed: invariant self-check W1 violations=$w1 W2 violations=$w2"
if [[ "$w1" != "0" || "$w2" != "0" ]]; then
  echo "seed: FAILED — fixture violates wallet invariants; refusing to proceed" >&2
  exit 1
fi

seed_psql -c "SELECT 'users' AS t, count(*) FROM users UNION ALL SELECT 'wallets', count(*) FROM wallets UNION ALL SELECT 'wallet_transactions', count(*) FROM wallet_transactions UNION ALL SELECT 'payment_orders', count(*) FROM payment_orders UNION ALL SELECT 'usage_logs', count(*) FROM usage_logs UNION ALL SELECT 'charge_lines', count(*) FROM charge_lines UNION ALL SELECT 'provider_evidence', count(*) FROM provider_evidence ORDER BY 1"
seed_psql -c "SELECT id, balance, frozen, version FROM wallets ORDER BY id"
echo "seed: OK database=$seed_db ready for backup_db.sh + restore_drill.sh"

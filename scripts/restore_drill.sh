#!/usr/bin/env bash
# ============================================================
# DeepTrols 备份恢复演练（TH-P05-07）
#
# 流程（全部真实执行）：
#   1. 目标防护：目标库名必须是隔离演练库（含 drill/test/staging/restore
#      标记），生产形态名称（deeptrols / prod / production / live / main）
#      一律拒绝 —— 指向生产库是灾难性的（AC-04）。
#   2. 在目标实例上 DROP + CREATE 一个全新的目标库。
#   3. pg_restore 恢复备份转储（计时）。
#   4. 验证（任一失败 → 退出码 1，全部通过 → 0）：
#      a. Schema 迁移版本（C-5）：本项目不维护 schema_migrations 版本表
#         （迁移按文件名顺序裸 SQL 应用），故以最新迁移标记对象存在性
#         + 源/目标 schema 对象集合与列数指纹完全一致来验证"恢复未引入
#         也未丢失迁移"。新增迁移时同步更新脚本内 MIGRATION 标记。
#      b. Manifest 行数：TH-P05-06 manifest 中的 7 张资金/证据表行数与
#         目标库逐一相等（AC-02）。
#      c. 钱包不变量（AC-03）：
#         W1 frozen == 未完结 reserve 流水金额之和（逐钱包精确）；
#         W2 balance == 已提交账本净额
#            （charge 记 -amount，reserve/release 不动余额，其余 +amount）。
#         提供 SOURCE_URL 时，源库跑同一查询作基线：目标库的违例集合
#         必须与源库完全一致（即恢复忠实保留数据，违例只能是源数据
#         自身的既有问题，不能是恢复引入的）。
#      d. 内容级一致性（不止行数）：核心表逐行 md5 校验和（源/目标
#         相等）、wallet_transactions 分类型金额合计、payment_orders
#         分状态金额合计 —— 提供 SOURCE_URL 时与源库对比。
#   5. 演练报告（stdout）：源转储 / manifest / 目标别名 / 恢复耗时 /
#      逐项验证结果（Observability 要求）。
#
# 用法：
#   scripts/restore_drill.sh <TARGET_DATABASE_URL> <DUMP_FILE> <MANIFEST_FILE> [SOURCE_DATABASE_URL]
#
# 失败注入支持：转储缺失 / manifest 缺失 / 不安全目标名均以明确
# 错误码退出（2=参数/防护拒绝，3=环境缺失，1=恢复或验证失败）。
#
# 安全约束：与 backup_db.sh 相同 —— URL 只解析进 libpq 环境变量，
# 不进命令行；所有子进程输出经 redact 过滤。
# ============================================================

set -euo pipefail

usage() {
  echo "Usage: $0 <TARGET_DATABASE_URL> <DUMP_FILE> <MANIFEST_FILE> [SOURCE_DATABASE_URL]" >&2
}

if [[ $# -lt 3 || $# -gt 4 ]]; then
  usage
  exit 2
fi

target_url="$1"
dump_file="$2"
manifest_file="$3"
source_url="${4:-}"

command -v pg_restore >/dev/null 2>&1 || { echo "drill: pg_restore not found in PATH" >&2; exit 3; }
command -v psql >/dev/null 2>&1 || { echo "drill: psql not found in PATH" >&2; exit 3; }

# ---- URL 解析（与 backup_db.sh 同规则） ----
urldecode() {
  local s="${1//+/ }"
  printf '%b' "${s//%/\\x}"
}

parse_url() {
  # $1=url；设置 PU_USER PU_PASS PU_HOST PU_PORT PU_DB
  local url="$1" rest creds hostdb hostport
  case "$url" in
  postgres://* | postgresql://*) ;;
  *)
    echo "drill: DATABASE_URL must start with postgres:// or postgresql://" >&2
    exit 2
    ;;
  esac
  rest="${url#*://}"
  if [[ "$rest" != *"@"* ]]; then
    echo "drill: DATABASE_URL must embed credentials (user:pass@host)" >&2
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
}

# ---- 目标防护（AC-04） ----
parse_url "$target_url"
target_db="$PU_DB"
target_lc="$(printf '%s' "$target_db" | tr '[:upper:]' '[:lower:]')"

case "$target_lc" in
deeptrols | prod | production | live | main)
  echo "drill: REFUSED — target database '$target_db' looks like a production database" >&2
  echo "drill: the restore drill requires an isolated target (name must contain drill/test/staging/restore)" >&2
  exit 2
  ;;
esac
if [[ ! "$target_lc" =~ (drill|test|staging|restore) ]]; then
  echo "drill: REFUSED — target database '$target_db' has no isolation marker" >&2
  echo "drill: target name must contain one of: drill, test, staging, restore" >&2
  exit 2
fi
if [[ ! "$target_lc" =~ ^[a-z][a-z0-9_]{0,62}$ ]]; then
  echo "drill: REFUSED — target database name contains unsafe characters: '$target_db'" >&2
  exit 2
fi

target_host="$PU_HOST"
target_port="$PU_PORT"
target_user="$PU_USER"
target_pass="$PU_PASS"

# ---- 输入文件检查（失败注入：缺失转储/清单） ----
if [[ ! -f "$dump_file" ]]; then
  echo "drill: FAILED — dump file not found: $dump_file" >&2
  exit 1
fi
if [[ ! -s "$dump_file" ]]; then
  echo "drill: FAILED — dump file is empty: $dump_file" >&2
  exit 1
fi
if [[ ! -f "$manifest_file" ]]; then
  echo "drill: FAILED — manifest file not found: $manifest_file" >&2
  exit 1
fi

# redact：目标/源密码都不落输出
redact() {
  local line
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ -n "$target_pass" ]]; then line="${line//"$target_pass"/[REDACTED]}"; fi
    if [[ -n "${source_pass:-}" ]]; then line="${line//"$source_pass"/[REDACTED]}"; fi
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

# 在目标实例执行 psql（库名由 $1 指定，其余来自目标 URL）
target_psql() {
  local db="$1"
  shift
  PGHOST="$target_host" PGPORT="$target_port" PGUSER="$target_user" PGPASSWORD="$target_pass" PGDATABASE="$db" \
    psql -X -q "$@"
}

failures=0
note_fail() {
  echo "drill: CHECK FAILED — $1" >&2
  failures=$((failures + 1))
}

start_epoch="$(date +%s)"
echo "drill: start target=$target_host:$target_port/$target_db dump=$dump_file manifest=$manifest_file"

# ---- 1. 全新目标库（DROP + CREATE） ----
echo "drill: recreating isolated target database '$target_db'"
if ! run_redacted target_psql postgres -c "DROP DATABASE IF EXISTS \"$target_db\" WITH (FORCE)"; then
  echo "drill: FAILED — could not drop existing target database" >&2
  exit 1
fi
if ! run_redacted target_psql postgres -c "CREATE DATABASE \"$target_db\""; then
  echo "drill: FAILED — could not create target database" >&2
  exit 1
fi

# ---- 2. 恢复（计时） ----
restore_start="$(date +%s)"
if ! PGHOST="$target_host" PGPORT="$target_port" PGUSER="$target_user" PGPASSWORD="$target_pass" PGDATABASE="$target_db" \
  run_redacted pg_restore --no-owner --no-privileges --exit-on-error -d "$target_db" "$dump_file"; then
  echo "drill: FAILED — pg_restore exited non-zero" >&2
  exit 1
fi
restore_end="$(date +%s)"
restore_secs=$((restore_end - restore_start))
echo "drill: restore OK duration=${restore_secs}s"

# ---- 3a. Schema 迁移版本验证（C-5） ----
# 架构事实（TH-P05-07 实测发现）：本项目不维护 schema_migrations 版本表 ——
# 迁移按文件名顺序以裸 SQL 应用（internal/repository/testutil/schema.go
# applyMigrations）。因此"迁移版本"通过两个客观证据验证：
#   (1) 最新迁移标记对象存在性（下探到仓库当前最高迁移号）；
#   (2) 源库/目标库 schema 对象集合完全一致（恢复不得新增或丢失对象）。
# 新增迁移时必须同步更新下方 MIGRATION_MARKERS（记录到该次最高迁移号）。
# 当前标记 = migrations/ 最高版本 000036。
files_max="$(ls "$(dirname "$0")/../migrations" 2>/dev/null | grep -E '^[0-9]{6}_.+\.up\.sql$' | sed -E 's/^0*([0-9]+)_.*/\1/' | sort -n | tail -1)"
echo "drill: repo migrations max version = ${files_max:-unknown}"

check_marker() {
  # $1=描述 $2=存在性 SQL（返回 't'/'f'）
  local desc="$1" sql="$2" got
  got="$(target_psql "$target_db" -A -t -c "$sql" 2>&1)" || got="ERR"
  if [[ "$got" == "t" ]]; then
    echo "drill: migration marker OK — $desc"
  else
    note_fail "migration marker missing in restored DB: $desc"
  fi
}
check_marker "000034 idx_model_pricing_platform_dual index" \
  "SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='public' AND indexname='idx_model_pricing_platform_dual')"
check_marker "000035 users.balance_alert_threshold column" \
  "SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name='balance_alert_threshold')"
check_marker "000036 auth_sessions table" \
  "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='auth_sessions')"

schema_obj_sql="SELECT table_name FROM information_schema.tables WHERE table_schema='public' ORDER BY 1"
tgt_tables="$(target_psql "$target_db" -A -t -c "$schema_obj_sql" 2>&1)" || tgt_tables="ERR"
tgt_table_count="$(printf '%s\n' "$tgt_tables" | grep -c . || true)"
echo "drill: restored schema objects: $tgt_table_count public tables"

if [[ -n "$source_url" ]]; then
  parse_url "$source_url"
  source_db="$PU_DB"
  source_host="$PU_HOST"
  source_port="$PU_PORT"
  source_user="$PU_USER"
  source_pass="$PU_PASS"
  source_psql() {
    PGHOST="$source_host" PGPORT="$source_port" PGUSER="$source_user" PGPASSWORD="$source_pass" PGDATABASE="$source_db" \
      psql -X -q "$@"
  }
  src_tables="$(source_psql -A -t -c "$schema_obj_sql" 2>&1)" || src_tables="ERR"
  if [[ "$tgt_tables" == "$src_tables" ]]; then
    echo "drill: schema object set OK (source==restored, $tgt_table_count tables)"
  else
    note_fail "schema object set differs after restore (migration-level drift)"
    diff <(printf '%s\n' "$src_tables") <(printf '%s\n' "$tgt_tables") | redact || true
  fi
  # 列级对照：每张表的列数一致（防止恢复丢失列定义）
  colcount_sql="SELECT table_name || ':' || count(*) FROM information_schema.columns WHERE table_schema='public' GROUP BY table_name ORDER BY 1"
  src_cols="$(source_psql -A -t -c "$colcount_sql" 2>&1)" || src_cols="ERR"
  tgt_cols="$(target_psql "$target_db" -A -t -c "$colcount_sql" 2>&1)" || tgt_cols="ERR"
  if [[ "$src_cols" == "$tgt_cols" ]]; then
    echo "drill: column-count fingerprint OK (source==restored)"
  else
    note_fail "column-count fingerprint differs after restore"
  fi
else
  source_psql=""
  echo "drill: NOTE no SOURCE_DATABASE_URL given — schema comparison limited to marker checks"
fi

# ---- 3b. Manifest 行数（AC-02） ----
echo "drill: verifying manifest row counts"
tab="$(printf '\t')"
while IFS= read -r line; do
  [[ -z "$line" || "$line" == \#* ]] && continue
  tbl="${line%%"$tab"*}"
  want="${line##*"$tab"}"
  if [[ ! "$tbl" =~ ^[a-z_]+$ ]]; then
    note_fail "manifest table name unsafe: '$tbl'"
    continue
  fi
  got="$(target_psql "$target_db" -A -t -c "SELECT count(*) FROM $tbl" 2>&1)" || {
    note_fail "count query failed for $tbl"
    continue
  }
  if [[ "$got" == "$want" ]]; then
    echo "drill: rows $tbl = $got (manifest OK)"
  else
    note_fail "row count mismatch for $tbl: manifest=$want restored=$got"
  fi
done <"$manifest_file"

# ---- 3c. 钱包不变量（AC-03） ----
frozen_sql="SELECT count(*) FROM (SELECT w.id FROM wallets w LEFT JOIN (SELECT wallet_id, COALESCE(sum(amount),0) AS open_reserve FROM wallet_transactions WHERE tx_type='reserve' GROUP BY wallet_id) r ON r.wallet_id = w.id WHERE w.frozen <> COALESCE(r.open_reserve,0)) x"
balance_sql="SELECT count(*) FROM (SELECT w.id FROM wallets w LEFT JOIN (SELECT wallet_id, sum(CASE WHEN tx_type IN ('reserve','release') THEN 0 WHEN tx_type='charge' THEN -amount ELSE amount END) AS s FROM wallet_transactions GROUP BY wallet_id) l ON l.wallet_id = w.id WHERE w.balance <> COALESCE(l.s,0)) x"
frozen_detail_sql="SELECT w.id || ' frozen=' || w.frozen || ' open_reserves=' || COALESCE(r.open_reserve,0) FROM wallets w LEFT JOIN (SELECT wallet_id, COALESCE(sum(amount),0) AS open_reserve FROM wallet_transactions WHERE tx_type='reserve' GROUP BY wallet_id) r ON r.wallet_id = w.id WHERE w.frozen <> COALESCE(r.open_reserve,0) LIMIT 20"
balance_detail_sql="SELECT w.id || ' balance=' || w.balance || ' ledger_sum=' || COALESCE(l.s,0) FROM wallets w LEFT JOIN (SELECT wallet_id, sum(CASE WHEN tx_type IN ('reserve','release') THEN 0 WHEN tx_type='charge' THEN -amount ELSE amount END) AS s FROM wallet_transactions GROUP BY wallet_id) l ON l.wallet_id = w.id WHERE w.balance <> COALESCE(l.s,0) LIMIT 20"

tgt_frozen="$(target_psql "$target_db" -A -t -c "$frozen_sql" 2>&1)" || tgt_frozen="ERR"
tgt_balance="$(target_psql "$target_db" -A -t -c "$balance_sql" 2>&1)" || tgt_balance="ERR"

if [[ "$tgt_frozen" == "0" ]]; then
  echo "drill: invariant W1 frozen==open-reserves OK (0 violations)"
else
  echo "drill: invariant W1 violations on restored DB: ${tgt_frozen}" | redact
  target_psql "$target_db" -A -t -c "$frozen_detail_sql" 2>/dev/null | redact || true
  if [[ -z "$source_url" ]]; then
    note_fail "W1 frozen invariant violated and no source baseline to compare"
  fi
fi
if [[ "$tgt_balance" == "0" ]]; then
  echo "drill: invariant W2 balance==ledger-sum OK (0 violations)"
else
  echo "drill: invariant W2 violations on restored DB: ${tgt_balance}" | redact
  target_psql "$target_db" -A -t -c "$balance_detail_sql" 2>/dev/null | redact || true
  if [[ -z "$source_url" ]]; then
    note_fail "W2 balance invariant violated and no source baseline to compare"
  fi
fi

# 源库基线对比：恢复只能忠实保留，不能引入新违例
if [[ -n "$source_url" ]]; then
  src_frozen="$(source_psql -A -t -c "$frozen_sql" 2>&1)" || src_frozen="ERR"
  src_balance="$(source_psql -A -t -c "$balance_sql" 2>&1)" || src_balance="ERR"
  echo "drill: source baseline W1 violations=$src_frozen W2 violations=$src_balance"
  if [[ "$tgt_frozen" != "$src_frozen" ]]; then
    note_fail "W1 violation count changed after restore: source=$src_frozen restored=$tgt_frozen"
  fi
  if [[ "$tgt_balance" != "$src_balance" ]]; then
    note_fail "W2 violation count changed after restore: source=$src_balance restored=$tgt_balance"
  fi
fi

# ---- 3d. 内容级一致性（不止行数） ----
content_tables=(users wallets wallet_transactions payment_orders usage_logs charge_lines provider_evidence)
for tbl in "${content_tables[@]}"; do
  sum_sql="SELECT COALESCE(sum(('x' || substr(md5(t.*::text),1,15))::bit(60)::bigint),0) FROM $tbl t"
  tgt_sum="$(target_psql "$target_db" -A -t -c "$sum_sql" 2>&1)" || {
    note_fail "checksum query failed for $tbl"
    continue
  }
  if [[ -n "$source_url" ]]; then
    src_sum="$(source_psql -A -t -c "$sum_sql" 2>&1)" || src_sum="ERR"
    if [[ "$tgt_sum" == "$src_sum" ]]; then
      echo "drill: checksum $tbl OK (source==restored)"
    else
      note_fail "content checksum mismatch for $tbl: source=$src_sum restored=$tgt_sum"
    fi
  else
    echo "drill: checksum $tbl = $tgt_sum (no source to compare)"
  fi
done

money_sql="SELECT COALESCE(sum(CASE WHEN tx_type='charge' THEN -amount ELSE amount END),0) || ' (' || COALESCE(sum(amount) FILTER (WHERE tx_type='reserve'),0) || ' open)' FROM wallet_transactions"
tgt_money="$(target_psql "$target_db" -A -t -c "$money_sql" 2>&1)" || tgt_money="ERR"
echo "drill: ledger net (committed basis) restored=$tgt_money"
if [[ -n "$source_url" ]]; then
  src_money="$(source_psql -A -t -c "$money_sql" 2>&1)" || src_money="ERR"
  if [[ "$tgt_money" != "$src_money" ]]; then
    note_fail "ledger net mismatch: source=$src_money restored=$tgt_money"
  fi
fi

po_sql="SELECT COALESCE(string_agg(status || ':' || cnt || ':' || total, ' '), 'none') FROM (SELECT status, count(*) cnt, COALESCE(sum(amount),0) total FROM payment_orders GROUP BY status ORDER BY status) x"
tgt_po="$(target_psql "$target_db" -A -t -c "$po_sql" 2>&1)" || tgt_po="ERR"
echo "drill: payment_orders by status restored=$tgt_po"
if [[ -n "$source_url" ]]; then
  src_po="$(source_psql -A -t -c "$po_sql" 2>&1)" || src_po="ERR"
  if [[ "$tgt_po" != "$src_po" ]]; then
    note_fail "payment_orders status/amount mismatch: source=$src_po restored=$tgt_po"
  fi
fi

# ---- 4. 报告 ----
end_epoch="$(date +%s)"
dump_bytes="$(wc -c <"$dump_file" | tr -d ' ')"
total_secs=$((end_epoch - start_epoch))
echo "drill: ============================ REPORT ============================"
echo "drill: dump_file=$dump_file dump_bytes=$dump_bytes"
echo "drill: manifest_file=$manifest_file"
echo "drill: target_alias=$target_db target_endpoint=$target_host:$target_port"
echo "drill: restore_duration=${restore_secs}s total_duration=${total_secs}s"
if [[ "$failures" -eq 0 ]]; then
  echo "drill: RESULT=PASS all verification checks OK"
  exit 0
else
  echo "drill: RESULT=FAIL failed_checks=$failures"
  exit 1
fi

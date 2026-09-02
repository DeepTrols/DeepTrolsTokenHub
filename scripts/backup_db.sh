#!/usr/bin/env bash
# ============================================================
# DeepTrols 数据库备份基线（TH-P05-06）
#
# 产出（按顺序）：
#   1. pg_dump -Fc 全量转储        -> <OUTPUT_DIR>/deeptrols_<UTC时间戳>.dump
#   2. 资金/证据 7 表行数 manifest -> <OUTPUT_DIR>/deeptrols_<UTC时间戳>.manifest
#      （原子写入：任何失败都不产生 manifest，杜绝假 manifest）
#   3. 报告（stdout）：耗时 / 转储字节数 / dump 与 manifest 路径
#
# 安全约束：
#   - DATABASE_URL 解析为 libpq 环境变量（PGHOST/PGPORT/PGUSER/
#     PGPASSWORD/PGDATABASE），URL 与密码永远不出现在进程命令行（argv）里
#   - 所有子进程输出统一经 redact 过滤后才打印（密码 -> [REDACTED]）
#   - 凭据含 URL 转义（%XX）时会先解码；不支持凭据内嵌 '@'
#
# 用法：
#   scripts/backup_db.sh <DATABASE_URL> [OUTPUT_DIR]   # OUTPUT_DIR 默认 ./backups
#
# 退出码：0 成功；2 参数错误；3 环境缺失；1 备份/manifest 失败
# ============================================================

set -euo pipefail

# 资金/证据链核心表（TH-P05-06 AC-02 固定清单，勿随意增删）
MANIFEST_TABLES=(users wallets wallet_transactions payment_orders usage_logs charge_lines provider_evidence)

usage() {
  echo "Usage: $0 <DATABASE_URL> [OUTPUT_DIR]" >&2
}

if [[ $# -lt 1 || $# -gt 2 ]]; then
  usage
  exit 2
fi

url="$1"
outdir="${2:-backups}"

command -v pg_dump >/dev/null 2>&1 || { echo "backup: pg_dump not found in PATH" >&2; exit 3; }
command -v psql >/dev/null 2>&1 || { echo "backup: psql not found in PATH" >&2; exit 3; }

case "$url" in
postgres://* | postgresql://*) ;;
*)
  echo "backup: DATABASE_URL must start with postgres:// or postgresql://" >&2
  exit 2
  ;;
esac

# ---- URL 解析（结果只进 libpq 环境变量，不进命令行） ----
urldecode() {
  local s="${1//+/ }"
  printf '%b' "${s//%/\\x}"
}

rest="${url#*://}"
if [[ "$rest" != *"@"* ]]; then
  echo "backup: DATABASE_URL must embed credentials (user:pass@host)" >&2
  exit 2
fi
creds="${rest%%@*}"
hostdb="${rest#*@}"
hostdb="${hostdb%%\?*}" # 去掉 ?sslmode=... 查询串
if [[ "$creds" == *":"* ]]; then
  PGUSER="$(urldecode "${creds%%:*}")"
  PGPASSWORD="$(urldecode "${creds#*:}")"
else
  PGUSER="$(urldecode "$creds")"
  PGPASSWORD=""
fi
hostport="${hostdb%%/*}"
PGDATABASE="${hostdb#*/}"
PGHOST="${hostport%%:*}"
if [[ "$hostport" == *":"* ]]; then
  PGPORT="${hostport##*:}"
else
  PGPORT=5432
fi
export PGUSER PGPASSWORD PGDATABASE PGHOST PGPORT

if [[ -z "$PGHOST" || -z "$PGDATABASE" || -z "$PGUSER" ]]; then
  echo "backup: could not parse host/database/user from DATABASE_URL" >&2
  exit 2
fi

pass="$PGPASSWORD"

# redact：从 stdin 读取，把任何密码出现替换为 [REDACTED]
redact() {
  if [[ -z "$pass" ]]; then
    cat
    return
  fi
  local line
  while IFS= read -r line || [[ -n "$line" ]]; do
    printf '%s\n' "${line//"$pass"/[REDACTED]}"
  done
}

# run_redacted：执行命令，合并 stdout/stderr，redact 后打印；返回原始退出码
run_redacted() {
  local out rc=0
  out="$("$@" 2>&1)" || rc=$?
  if [[ -n "$out" ]]; then
    printf '%s\n' "$out" | redact
  fi
  return "$rc"
}

mkdir -p "$outdir"
ts="$(date -u +%Y%m%dT%H%M%SZ)"
dump_file="$outdir/deeptrols_${ts}.dump"
manifest_file="$outdir/deeptrols_${ts}.manifest"
manifest_tmp="${manifest_file}.tmp"

cleanup() { rm -f "$manifest_tmp"; }
trap cleanup EXIT

start_epoch="$(date +%s)"

echo "backup: start host=$PGHOST port=$PGPORT db=$PGDATABASE user=$PGUSER"

# ---- 1. 全量转储（与 docs/DEPLOYMENT.md 基线命令同语义：-Fc） ----
if ! run_redacted pg_dump -Fc -f "$dump_file"; then
  echo "backup: FAILED pg_dump exited non-zero; no manifest written" >&2
  rm -f "$dump_file"
  exit 1
fi

if [[ ! -s "$dump_file" ]]; then
  echo "backup: FAILED dump file is empty: $dump_file" >&2
  rm -f "$dump_file"
  exit 1
fi

# ---- 2. Manifest：固定 7 表行数；一条 SQL，全成功才原子落盘 ----
query=""
for t in "${MANIFEST_TABLES[@]}"; do
  if [[ -n "$query" ]]; then query+=" UNION ALL "; fi
  query+="SELECT '$t', count(*) FROM $t"
done
query+=" ORDER BY 1;"

tab="$(printf '\t')"
if ! counts="$(run_redacted psql -X -q -A -t -F "$tab" -c "$query")"; then
  echo "backup: FAILED row-count query failed; no manifest written" >&2
  exit 1
fi

{
  echo "# deeptrols backup manifest (TH-P05-06)"
  echo "# generated_at=$ts"
  echo "# database=$PGDATABASE host=$PGHOST port=$PGPORT"
  echo "# dump_file=deeptrols_${ts}.dump"
  printf '%s\n' "$counts"
} >"$manifest_tmp"
mv "$manifest_tmp" "$manifest_file"

# ---- 3. 报告：耗时 / 文件大小 / 路径（observability 要求） ----
end_epoch="$(date +%s)"
size_bytes="$(wc -c <"$dump_file" | tr -d ' ')"
echo "backup: OK duration=$((end_epoch - start_epoch))s dump_bytes=$size_bytes dump_path=$dump_file manifest_path=$manifest_file"

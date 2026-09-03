# DeepTrols 生产部署手册

> 目标读者：负责把 DeepTrols 部署到生产环境的 SRE / 后端工程师。
> 本文是发布清单（runbook），不是架构文档。架构见 `docs/AI聚合网关_完整文档.md`。

## 0. 上线前置条件（必须全部满足）

- [ ] 已接入支付通道：当前为易支付（epay）M0（下单/回调验签 + 幂等）。`ENABLE_FAKE_PAYMENT=false` 时演示充值/兑换码/注册送余额全部关闭。官方支付渠道（支付宝/微信）接入前，仅允许内部 Beta，不允许公开收费。
- [ ] 生产配置基线通过启动校验：config 在 `ENABLE_FAKE_PAYMENT=false` 时强制
      `COOKIE_SECURE=true`、`ADMIN_PASSWORD` ≥ 12 字节、所有密钥非弱默认值，
      不满足则拒绝启动（fail-fast）。
- [ ] 至少一个实例的 Worker 已启用（健康检查、对账、订阅过期/自动续费）。多实例部署必须配合 Redis
      lease（见「多实例注意事项」）。
- [ ] TLS 在反向代理 / 云 LB 终止，后端只监听内网回环。

## 1. 环境变量基线

| 变量 | 生产要求 | 生成方式 |
|---|---|---|
| `DATABASE_URL` | 强密码 + SSL（`sslmode=require` 或云厂商 CA） | 数据库控制台 |
| `REDIS_URL` | 启用密码，走内网/TLS | `redis-cli CONFIG SET requirepass ...` |
| `JWT_SECRET` | ≥ 32 字节强随机，泄露=全站会话伪造 | `openssl rand -hex 32` |
| `ENCRYPTION_KEY` | 恰好 32 字节强随机，泄露=全部 API Key 明文暴露 | `openssl rand -hex 16` |
| `ADMIN_PASSWORD` | ≥ 12 字节强密码，首登后立即轮换 | 密码管理器生成 |
| `COOKIE_SECURE` | `true`（强制） | — |
| `COOKIE_SAMESITE` | `Strict`（默认） | — |
| `ENABLE_FAKE_PAYMENT` | `false`（强制） | — |
| `CORS_ORIGIN` | 仅前端正式域名 | — |
| `API_HOST` | 生产建议绑定内网回环（默认 `0.0.0.0`） | — |
| `LOAD_TTL_SECONDS` | 渠道实时负载计数器 TTL（默认 60） | — |
| `JWT_EXPIRY_HOURS` | 登录态有效期（默认 24） | — |
| `WORKER_METRICS_ADDR` | Worker `/metrics` 监听地址（默认 `127.0.0.1:19090` 仅回环；置空禁用端点，仅关闭可观测性，worker 照常运行）（TH-P05-11） | — |

生产校验由 `internal/config/config.go` 的 `validate()` 执行；任何不满足的配置都会在进程启动时报错退出，部署流水线应把「启动即退出」视为失败。

## 2. 构建与启动

```bash
go build -trimpath -ldflags "-s -w" -o bin/api ./cmd/api
go build -trimpath -ldflags "-s -w" -o bin/worker ./cmd/worker

# 以 systemd / 容器管理工具托管，至少一个 worker 进程
./bin/api &
./bin/worker &
```

健康检查：

- 存活探针：`GET /health`（200 且 `{"status":"ok"}`）
- 就绪探针：`GET /readyz`（校验 DB / Redis 连通性；Redis 未配置时仅校验 DB）

## 3. 数据库迁移

> **现状（TH-P05-08 于 2026-09-03 在空库上实际验证）**：本仓库**不维护
> `schema_migrations` 版本表**，迁移按文件名顺序的裸 SQL 应用（与 §5.3
> 恢复演练的口径一致）。当前迁移版本 = 最新文件名
> （`000036_auth_sessions`）。`Makefile` 的 `migrate-up/down` 依赖
> `golang-migrate` CLI（会创建版本表）——两种路径不可混用；未装该 CLI
> 的环境按下面的 psql 顺序执行。

空库全量应用（每个文件单事务、失败即停）：

```bash
# 升级（先备份）：扩展 + 按文件名顺序逐个应用
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1 \
  -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp";' \
  -c 'CREATE EXTENSION IF NOT EXISTS "pgcrypto";'
for f in $(ls migrations/*.up.sql | sort); do
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1 -q -f "$f" || { echo "FAILED: $f"; exit 1; }
done
```

回滚一个版本（仅发布回滚场景；先在一次性库演练对应 `.down.sql`）：

```bash
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1 -f migrations/<最新版本>.down.sql
```

若环境装有 `golang-migrate` CLI，也可用
`migrate -path migrations -database "$DATABASE_URL" up|down 1`，
但其版本表与裸 SQL 路径互不兼容，同一库只能固定走其中一条路径。

**dirty / 半截迁移修复（事故处理，裸 SQL 路径）**：

1. 对照 `migrations/` 文件名顺序与实际表结构，确认最后完整应用到哪一版。
2. 半截文件：手工补齐缺失 DDL 或回滚已执行的半截（单事务内）。
3. 记录事故原因到 `docs/PROJECT_STATUS.md`。

> 历史事故：2025 年（golang-migrate 时期）曾因并发/中断出现
> `schema_migrations` dirty，靠 `migrate force 8` 现场修复。迁移操作必须
> 串行执行、避开流量高峰。

## 4. 密钥轮换

- `JWT_SECRET`：滚动发布（新值生效后旧 token 全部失效，通知用户重新登录）。
- `ENCRYPTION_KEY`：当前实现不支持多密钥解密，轮换会破坏已存储的 API Key。
  如需轮换，必须提供重新加密数据的迁移工具后再操作。
- 各渠道 `api_key`：在下游渠道侧轮换后更新库内配置。

## 5. 备份与恢复

### 5.1 基线备份命令（TH-P05-06）

```bash
# 全量转储 + 资金/证据表行数 manifest + 报告（耗时/字节数/路径）
# 产出：<OUTPUT_DIR>/deeptrols_<UTC时间戳>.dump 与同名 .manifest
scripts/backup_db.sh "$DATABASE_URL" /var/backups/deeptrols
```

脚本保证（与手工 `pg_dump "$DATABASE_URL" -Fc -f ...` 同语义，但增加安全与校验）：

- DATABASE_URL 解析为 libpq 环境变量，URL/密码不出现在进程命令行；
  所有子进程输出经 redact 过滤后才打印（输出不含 DB 密码）。
- manifest 记录 `users / wallets / wallet_transactions / payment_orders /
  usage_logs / charge_lines / provider_evidence` 7 张资金/证据表的行数；
  原子写入：任何失败（无效 URL、连不上、缺表、空转储）都以非零退出且
  不产生 manifest（杜绝假 manifest）。
- 退出码：0 成功；1 备份/manifest 失败；2 参数错误；3 环境缺 `pg_dump`/`psql`。

定时执行（示例，错峰）：

```bash
17 3 * * * /opt/deeptrols/scripts/backup_db.sh "$DATABASE_URL" /var/backups/deeptrols >> /var/log/deeptrols-backup.log 2>&1
```

### 5.2 存储与加密预期

- 留存位置：本机 `/var/backups/deeptrols`（或等价目录）仅为落地点，
  必须再上传至批准的异地/对象存储；存储路径与保留策略由运维确认后记录在案。
- 加密：转储含全量业务数据，落地后、离开本机前必须加密
  （如 `age`/`gpg` 对称加密，或服务端加密的对象存储桶）。manifest 与
  dump 成对保存。
- 备份频率：至少每日；对账/计费库建议 WAL 归档。

### 5.3 恢复演练（TH-P05-07）

真实恢复必须走演练工具，**禁止**直接对生产库执行 `pg_restore`：

```bash
# 1. 用基线备份工具产生转储 + manifest（§5.1）
scripts/backup_db.sh "$DATABASE_URL" /var/backups/deeptrols

# 2. 在隔离目标库上执行演练（目标名必须含 drill/test/staging/restore 标记）
#    第 4 个参数传源库 URL 时，启用源/目标内容级对比（强烈推荐）
scripts/restore_drill.sh "$RESTORE_TARGET_URL" <dump 文件> <manifest 文件> "$DATABASE_URL"
```

工具执行流程（全部自动）：目标防护 → DROP+CREATE 隔离目标库 →
`pg_restore`（计时）→ 验证 → 报告。验证项：

- **迁移版本（C-5）**：本项目不维护 `schema_migrations` 版本表（迁移按
  文件名顺序裸 SQL 应用），因此验证 = 最新迁移标记对象存在性 +
  源/目标 schema 对象集合与列数指纹完全一致。新增迁移后必须同步更新
  `scripts/restore_drill.sh` 内的 `MIGRATION_MARKERS`。
- **Manifest 行数**：7 张资金/证据表逐一等于 manifest 记录。
- **钱包不变量**：W1 `frozen == 未完结 reserve 之和`；W2 `balance ==
  账本净额`（charge 记 `-amount`，reserve/release 不动余额，其余 `+amount`）。
  提供源库时，目标库的违例集合必须与源库完全一致（恢复不得引入新违例）。
- **内容级一致性**：7 表逐行 md5 校验和、账本净额、分状态支付金额与源库相等。

报告输出（stdout）：转储路径/字节数、目标别名、恢复耗时、逐项验证结果。
演练通过后由操作人在报告中签字，结果记入 `docs/PROJECT_STATUS.md`；
每季度至少一次。

可选：用 `scripts/seed_drill_fixture.sh "$SEED_URL"` 在隔离库播种含完整
钱包生命周期（topup/reserve/commit/settle/release/transfer）的夹具，
使演练在非空账本上验证不变量，而不是在空表上空转。

**失败处理（按退出码）**：

| 退出码 | 含义 | 处置 |
|---|---|---|
| 2 | 参数错误 / 目标防护拒绝（生产形态库名、无隔离标记） | 换用隔离目标库；绝不可绕过防护 |
| 3 | 环境缺失（`pg_restore`/`psql` 不在 PATH） | 在装有 PostgreSQL 客户端的机器执行 |
| 1 | 恢复失败或任一验证项失败 | 保留现场：转储、manifest、完整 stdout/stderr；对比验证输出定位差异；确认转储与源库状态后重跑；仍失败则升级至 `docs/PROJECT_STATUS.md` 已知问题 |

真实生产事故的恢复（非演练）不在本工具范围：先恢复进隔离库演练验证，
再由运维决策切换，禁止未经演练直接恢复生产。

## 6. 多实例注意事项

- Worker（健康检查 / 对账 / 订阅过期与自动续费）**必须**启用 Redis lease（`internal/pkg/lease`）；
  否则多个 worker 实例会重复执行对账或订阅回收、产生冲突结果。
- API 无状态，可水平扩展；Redis 与 PostgreSQL 是共享状态。

### 6.1 Worker 可观测性预期（TH-P05-11）

- Worker 进程暴露独立的 `/metrics`（`WORKER_METRICS_ADDR`，默认
  `127.0.0.1:19090`），与 API 共用同一套受审指标家族与标签策略
  （`worker_cycles_total` / `worker_cycle_duration_seconds` /
  `worker_lease_total`，见 `docs/OBSERVABILITY_METRICS.md`）。
  Prometheus 需把**每个 worker 实例**都列入抓取目标；跨主机抓取时在网络层
  放行该端口（端点本身无鉴权，勿暴露到公网）。
- 多实例下每个 tick 只有一个实例记 `worker_lease_total{outcome="acquired"}`
  与 `worker_cycles_total{outcome="success"}`，其余实例记 `skipped`——这是
  租约竞争的正常形态，不是故障。聚合观测时按 `worker` 标签聚合（不带实例维度）。
- Redis 租约错误是 **fail-closed**：该周期跳过、业务函数不执行，
  `worker_lease_total{outcome="error"}` +1 且 `worker_cycles_total{outcome="failed"}` +1；
  进程继续运行，下一周期自动重试。`error` 持续增长即代表 Redis 故障，应优先排查。
- 指标端点绑定失败只降级可观测性（日志 `worker metrics server failed`），
  租约调度与业务不受影响；发现该日志时修复监听配置即可。

## 7. 灰度与回滚

- 灰度：API 新版本先在 1 个实例验证 `/health`、登录、一次真实网关调用，
  观察错误率与计费日志后再全量。
- 回滚：二进制回退到上一发布版本；若涉及新迁移，按第 3 节 `down 1` 回滚。
- 任何发布必须先在 staging 跑一遍 `go test ./... -count=1`（每包独立 schema，
  可并行）与迁移 up/down 往返。

## 8. 上线后第一周观察项

- 对账 L0/L1 跑批无差异、无残留 pending。
- 少收证据（`usage_logs.error_code = 'undercharged'`）由对账产出
  `undercharge_review`（severity=critical）差异记录，**进入人工复核**；
  系统任何路径（对账 / worker）都不会自动补扣钱包差额，
  手动调整仅能由 P5 复核任务（TH-P5-RVW-*）引入并留审计行。
  网关进程内计数（`UnderchargeFallbackCounts`，按 endpoint+model）
  上线初期应定期观测，持续增长说明存在定价或预留缺口。
- 订阅到期回收与自动续费任务按计划执行，无重复扣费。
- 非流式/流式失败请求全部能在 `usage_logs` 找到（`status=failed/partial`），
  不存在"账外请求"。
- 4xx 错误率、网关延迟 P95、Redis 命中率、DB 连接数处于基线内。
- Worker 周期指标处于基线（TH-P05-11）：60s 级 worker（`health_checker` /
  `billing_sync`）与 1h 级 worker（`reconciler` / `subscription_expirer` /
  `subscription_renewer`）的 `worker_cycles_total{outcome="success"}` 应按
  调度周期持续增长；`outcome="failed"` / `panic_recovered` 出现即排查，
  `worker_lease_total{outcome="error"}` 持续出现即 Redis 故障。
  沉默判据与告警基线由 TH-P05-05 定义（见 `docs/OBSERVABILITY_METRICS.md` §5）。
- 若发现异常，按 `docs/PROJECT_STATUS.md` 的已知问题章节排查。

## 9. 空环境部署验证（TH-P05-08，2026-09-03 已执行一次）

在**全新空库 + 全新 Redis + 生产形态配置**（`ENABLE_FAKE_PAYMENT=false`、
`COOKIE_SECURE=true`、强随机密钥）上完整跑通：迁移全量应用（36 个文件）→
一次安全 down/up 往返 → API 启动（`/health` `/healthz` `/readyz` 200，
`/metrics` 可抓取）→ Worker 带 Redis lease 周期（`worker:lease:*`）→
播种测试模型/渠道后网关冒烟（请求→定价→预留→executor→结算，
证据链 `usage_logs`/`charge_lines`/`provider_evidence`/账本齐全，
扣费与定价精确一致）→ 重启后重放同一 `X-Request-ID` 无重复资金效果 →
对账运行产出 `reconciliation_runs` 行。缺环境变量 / 弱默认密钥 /
`COOKIE_SECURE=false` / 短管理员密码均**拒绝启动（fail-fast）**。

- 完整 Clean Deployment Matrix、证据摘录与记录在案的偏差见
  `docs/tasks/execution-logs/TH-P05-08.md`。
- 播种与对账的一次性工具：`scripts/p0508_clean_seed.go`、
  `scripts/p0508_recon_driver.go`（均 `//go:build ignore`，不进生产二进制）。
- 支付宝/微信官方支付尚未实现（P1 任务），无配置项即无启动依赖；
  `ENABLE_FAKE_PAYMENT=false` 时演示充值端点固定返回 403。

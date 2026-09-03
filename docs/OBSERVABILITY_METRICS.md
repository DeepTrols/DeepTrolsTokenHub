# Observability Metrics（TH-P05-04 基线 + TH-P05-11 Worker 扩展 + TH-P05-05 告警支持）

P0.5 生产可观测性基线：网关流量与资金路径（reserve / settle / release /
undercharge / pricing-incomplete / provider-before-call blocking）的最小
低基数 Prometheus 计数器；TH-P05-11 将同一 registry 与标签策略扩展到
worker 租约（lease）可观测性（周期结果 / 周期耗时 / 租约决策）；
TH-P05-05 仅追加告警包所需的最小支持家族（对账关键差异计数器、
依赖可达性 gauge），告警规则本体见 `ops/prometheus/tokenhub_p05.rules.yml`。

实现位置：`internal/pkg/metrics`（独立 registry + 依赖 watchdog）、
`internal/handler/middleware/gateway_metrics.go`（请求计数）、
`internal/service/billing/charger.go`（资金路径计数）、
`internal/handler/gateway`（fail-closed / provider-blocked 计数）、
`cmd/worker/main.go`（worker 周期 / 租约计数与 worker `/metrics` 端点）、
`internal/worker/reconciliation/reconciler.go`（critical 差异计数）、
`cmd/api/main.go` + `cmd/worker/main.go`（依赖 watchdog 启动）。

## 1. 指标清单

### 网关（Gateway）

| 指标名 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `gateway_requests_total` | Counter | `endpoint` | 网关请求总数（所有结果） |
| `gateway_success_total` | Counter | `endpoint` | 2xx/3xx 完成的请求 |
| `gateway_error_total` | Counter | `endpoint`, `reason_class` | 4xx/5xx 完成的请求 |
| `gateway_request_duration_seconds` | Histogram | `endpoint` | 请求耗时（buckets: 0.1/0.25/0.5/1/2/5/10/30/60/120s） |

### 资金路径（Billing / Money path）

| 指标名 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `billing_reserve_total` | Counter | — | 成功的钱包预扣（含幂等重放；资金效果唯一性由 TH-P05-03 W5 证明） |
| `billing_reserve_failed_total` | Counter | `reason_class` | 失败的预扣 |
| `billing_settle_total` | Counter | — | 成功按最终成本结算 |
| `billing_settle_failed_total` | Counter | `reason_class` | 被拒绝的结算 |
| `billing_release_total` | Counter | — | 成功的冻结释放（补偿） |
| `billing_release_failed_total` | Counter | `reason_class` | 失败的冻结释放 |
| `billing_undercharge_fallback_total` | Counter | `endpoint` | TH-P05-02 settle fallback 产生的少收证据事件 |
| `billing_pricing_incomplete_total` | Counter | `endpoint` | 定价不完整、在任何 reserve 之前 fail-closed 拒绝的请求 |

### Provider 安全（Surrogate）

| 指标名 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `gateway_provider_blocked_before_call_total` | Counter | `endpoint`, `reason_class` | 在任何上游请求发出之前被资金安全闸门拦截的 provider 调用 |

**为什么是 surrogate（代理指标）**：真实的 “provider 调用被阻止” 事件无法在
executor 内部计数（executor 不知道调用为何没发生）。但资金不变量 **W6**
（TH-P05-03 已用真实 Postgres 证明：每一笔 provider 调用都必须先有一次成功
的 reserve）保证了等价性——任何发生在 executor 之前的拒绝（`pricing_incomplete` /
`wallet_missing` / `insufficient_balance` / `reserve_failed`）都恰好等于一次被
阻止的 provider 调用。计数点位于各网关处理器的 fail-closed 分支。

### Worker 租约可观测性（TH-P05-11）

| 指标名 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `worker_cycles_total` | Counter | `worker`, `outcome` | worker 周期按结果计数（`success` / `failed` / `panic_recovered` / `skipped`） |
| `worker_cycle_duration_seconds` | Histogram | `worker` | 周期耗时（含租约获取；buckets: 0.05/0.1/0.25/0.5/1/2.5/5/10/30/60/120/300/600s） |
| `worker_lease_total` | Counter | `worker`, `outcome` | 租约决策（`acquired` / `skipped` / `error`）；Redis 错误为 fail-closed：周期被跳过 |

**语义**（租约协议本身由 `internal/pkg/lease` 提供，本批次不改变其行为）：

- 租约获取成功 → 执行业务函数 → `success`（业务报错则 `failed`，panic 被
  `runSafely` 的 recover 边界捕获则 `panic_recovered`）。
- 租约已被其他实例持有 → 本周期跳过（`skipped`），**绝不重复执行**。
- Redis 租约错误 → **fail-closed**：业务函数永不被调用，周期记为 `failed`，
  租约记为 `error`；进程继续运行，下一周期重试。
- `nil` Redis（单实例模式）→ 租约恒授予，行为与 TH-P05-11 之前完全一致。
- 每个周期恰好产生一次周期观测（四种结果之一）与一次耗时观测；
  指标故障由 `safely()` 吞掉，永远不会改变租约结果、重复执行 worker 或
  阻塞周期。

### 告警支持（TH-P05-05）

| 指标名 | 类型 | 标签 | 含义 |
|---|---|---|---|
| `reconciliation_critical_diffs_total` | Counter | — | 成功持久化的 severity=critical 对账差异（undercharge_review / error_mislabel）；warning 类差异永不计数 |
| `app_dependency_up` | Gauge | `dependency` | 1=可达 / 0=不可达；由进程内依赖 watchdog 每 15s 探测更新 |

**语义**：

- `reconciliation_critical_diffs_total` 只在差异行成功写入
  `reconciliation_diffs` 之后、且仅当 `severity="critical"` 时递增；它是
  对账“detect-only”语义的可观测投影——计数递增本身**从不**触发任何修正
  动作。TH-P05-08 类 warning 差异（如幂等重放缺计费行）不进入该计数器，
  因此不会对 critical 告警形成噪声。
- `app_dependency_up` 由 `StartDependencyWatchdog`（15s 间隔、2s 探测超时）
  驱动：API 与 worker 进程都启动该 watchdog；探测为 `Pool.Ping`
  （`dependency="database"`）与 `Redis.Ping`（`dependency="redis"`，仅在
  Redis 已配置时注册）。未配置 Redis 的单实例开发模式下对应序列不存在，
  Redis 告警永不触发。探测与置值均由 `safely()` 包裹：watchdog 故障只影响
  gauge，绝不影响业务请求或租约行为。
- 告警规则（`ops/prometheus/tokenhub_p05.rules.yml`，9 条）只消费本清单内
  的既有家族与上述两个支持家族；触发/恢复语义由
  `ops/prometheus/tests/p05_alerts_test.yml`（promtool fixture）固化，
  结构与安全策略由 `ops/prometheus/rules_test.go` 固化，处置手册为
  `docs/RUNBOOK_ALERTS.md`。

## 2. 标签策略（Label policy）

### 允许的低基数标签（仅此五个）

- `endpoint`：有界端点白名单。取值：
  `chat/completions`、`completions`、`responses`、`messages`、
  `messages/count_tokens`、`embeddings`、`images/generations`、`images/edits`、
  `audio/speech`、`audio/transcriptions`、`videos/generations`、`models`、
  `video`、`other`（钳制桶）。
  - `/v1/` 前缀在入白名单前剥离；计费调用点的短形式 `chat` 经
    `endpointAliases` 归一化为 `chat/completions`，保证同一端点只有一个标签值。
- `reason_class`：有界原因类。取值：
  `insufficient_balance`、`reserve_failed`、`pricing_incomplete`、
  `wallet_missing`、`tx_not_reserved`、`settle_error`、`client_error`、
  `server_error`、`other`（钳制桶）。
- `worker`（TH-P05-11）：有界 worker 白名单（`AllowedWorkers`）。取值：
  `health_checker`、`reconciler`、`billing_sync`、`subscription_expirer`、
  `subscription_renewer`、`other`（钳制桶）。新增租约 worker 必须**刻意**
  加入白名单；动态或未知名称（原始租约键、主机名、pod id 等）一律钳制为
  `other`，worker 身份不可能被走私进标签。
- `outcome`（TH-P05-11）：有界结果白名单。周期结果取值：
  `success`、`failed`、`panic_recovered`、`skipped`；租约决策取值：
  `acquired`、`skipped`、`error`；其他一律钳制为 `other`。
- `dependency`（TH-P05-05）：有界依赖白名单。取值：`database`、`redis`、
  `other`（钳制桶）。任何非白名单值（DSN 片段、host:port、连接串等）一律
  钳制为 `other`——依赖身份不可能携带凭据或地址进入标签。

### 结构性强制

每个 setter 都在写入 Prometheus 之前经过 `SanitizeEndpoint` /
`SanitizeReasonClass` / `SanitizeWorker` / `SanitizeCycleOutcome` /
`SanitizeLeaseOutcome` / `SanitizeDependency` 白名单消毒，未来的调用方
**不可能**把高基数或敏感值写进标签（单元测试
`TestGatheredFamilies_NoForbiddenLabels` 对 Gather 结果做结构断言，含恶意
输入的 worker / dependency setter 场景）。

### 禁止出现（指标名、标签名、标签值均不得包含）

`request_id`、`user_id`、`tenant_id`、`order_no`、email、API key、JWT、
prompt 文本、原始错误文本、原始 URL、IP 地址。

## 3. 运维

- API 进程：`/metrics` 由 `App.RegisterRoutes` 挂载（Prometheus 文本格式，
  独立 registry：只含上述受审家族，无 Go runtime / 第三方采集器）。
- Worker 进程（TH-P05-11）：`/metrics` 由 `serveWorkerMetrics` 提供，
  监听地址由环境变量 `WORKER_METRICS_ADDR` 控制，**默认 `127.0.0.1:19090`
  （仅回环）**；置空即禁用端点（仅关闭可观测性，worker 照常运行）；
  绑定失败只降级可观测性、不影响租约调度。跨主机抓取时在网络层放行该端口。
  该端点复用同一个 `metrics.Default` registry，因此 worker 家族与网关/资金
  家族使用同一套标签策略与暴露面约束；**不存在第二个 registry**。
- 与 `/health` 一样**无鉴权**；生产环境必须在网络层限制访问
  （内网 / firewall / scrape 专用网络），不要暴露到公网。
- 所有 setter 均由 `safely()` 包裹（recover 守卫）：**指标故障永远不会影响
  业务请求或租约结果**（TH-P05-04 硬性要求，测试
  `TestSafely_InstrumentationFailureNeverPropagates` 固化；TH-P05-11 要求
  指标故障绝不重复执行 worker、不改变租约语义）。
- 请求计数中间件在 handler 之后计数（`chimw.NewWrapResponseWriter`），
  且位于 `/v1` 路由链最前，保证鉴权/限流拒绝也被计入。
- 依赖 watchdog（TH-P05-05）：API 与 worker 进程内各运行一个
  `StartDependencyWatchdog`，每 15s 探测数据库 / Redis 连接并更新
  `app_dependency_up`；探测是独立协程，故障只影响 gauge，不阻塞业务或
  租约路径。生产环境两个进程都必须被 Prometheus 抓取（示例配置见
  `ops/prometheus/prometheus.example.yml`）——`min()` 聚合保证任一进程
  失联即触发依赖告警。

## 4. 范围边界

- Worker 租约（lease）指标已由 TH-P05-11 纳入（见 §1「Worker 租约可观测性」）。
- TH-P05-05 仅追加两个告警支持家族（见 §1「告警支持」）：没有日志 grep
  探针、没有完整数据库 exporter、没有第二套观测架构；告警规则只消费
  本清单内的指标。
- 不包含 Go runtime / process 采集器（刻意排除，保持暴露面最小）。

## 5. Worker Silence 基线（TH-P05-05 已消费）

TH-P05-11 提供**指标**，TH-P05-05 基于它们实现了分层沉默告警
（`ops/prometheus/tokenhub_p05.rules.yml` 中的三条 `TokenHubWorkerSilent*`
规则），无需新增时间戳指标：

- 每个租约 worker 的调度周期固定：`health_checker` / `billing_sync` 为
  60s tick；`reconciler` / `subscription_expirer` / `subscription_renewer`
  为 1h tick。注意差异：`subscription_expirer` / `subscription_renewer`
  有一次启动周期，**`reconciler` 没有启动周期**（以 `cmd/worker/main.go`
  实际调度代码为准；TH-P05-05 的分层窗口即据此推导）。
- 沉默判据采用两段式：`increase(...) == 0`（存活但停摆；increase 对计数器
  重置安全，进程重启不会误报）`or absent(...)`（进程死亡 / 指标从未导出，
  5m staleness 后触发）。
- 租约竞争场景下只有一个实例计入 `acquired`/`success`，沉默规则按
  `worker` 全集群聚合（不带实例维度）判断，健康的“另一实例被跳过”
  （`outcome="skipped"`）永不触发告警。
- 本批次**未**新增 `last_success_timestamp_seconds` 类 gauge：计数器+
  调度周期已足以构造上述规则（见 TH-P05-11 执行记录）。

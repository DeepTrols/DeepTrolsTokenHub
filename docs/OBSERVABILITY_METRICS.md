# Observability Metrics（TH-P05-04 基线）

P0.5 生产可观测性基线：网关流量与资金路径（reserve / settle / release /
undercharge / pricing-incomplete / provider-before-call blocking）的最小
低基数 Prometheus 计数器。

实现位置：`internal/pkg/metrics`（独立 registry）、
`internal/handler/middleware/gateway_metrics.go`（请求计数）、
`internal/service/billing/charger.go`（资金路径计数）、
`internal/handler/gateway`（fail-closed / provider-blocked 计数）。

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

## 2. 标签策略（Label policy）

### 允许的低基数标签（仅此两个）

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

### 结构性强制

每个 setter 都在写入 Prometheus 之前经过 `SanitizeEndpoint` /
`SanitizeReasonClass` 白名单消毒，未来的调用方**不可能**把高基数或敏感值
写进标签（单元测试 `TestGatheredFamilies_NoForbiddenLabels` 对 Gather 结果
做结构断言）。

### 禁止出现（指标名、标签名、标签值均不得包含）

`request_id`、`user_id`、`tenant_id`、`order_no`、email、API key、JWT、
prompt 文本、原始错误文本、原始 URL、IP 地址。

## 3. 运维

- `/metrics` 由 `App.RegisterRoutes` 挂载（Prometheus 文本格式，独立
  registry：只含上述受审家族，无 Go runtime / 第三方采集器）。
- 与 `/health` 一样**无鉴权**；生产环境必须在网络层限制访问
  （内网 / firewall / scrape 专用网络），不要暴露到公网。
- 所有 setter 均由 `safely()` 包裹（recover 守卫）：**指标故障永远不会影响
  业务请求**（TH-P05-04 硬性要求，测试
  `TestSafely_InstrumentationFailureNeverPropagates` 固化）。
- 请求计数中间件在 handler 之后计数（`chimw.NewWrapResponseWriter`），
  且位于 `/v1` 路由链最前，保证鉴权/限流拒绝也被计入。

## 4. 范围边界

- **不包含** worker 租约（lease）指标——属于 TH-P05-11 范围。
- 不包含 Go runtime / process 采集器（刻意排除，保持暴露面最小）。

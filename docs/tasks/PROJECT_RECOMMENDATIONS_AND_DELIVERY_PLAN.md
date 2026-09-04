# 智曜TokenHub 项目整体建议与可执行交付路线图

> 编制日期：2026-09-03
>
> 适用范围：`DeepTrolsTokenHub` 单仓库内的后端、控制台、`ai-nuxt`、harness 与现有 `docs/tasks/` Backlog
>
> 文档性质：执行建议与路线图，不替代各阶段任务文件中的 Scope、AC、Dependencies 和全局 DoD
>
> 原编制动作：仅新增本规划文档；2026-09-04 随目录合并更新仓库边界描述。下方任务进度是编制日快照，最新状态以 `TASK_INDEX.md` 和执行日志为准。

## 1. 结论

项目的计费、钱包、账本、网关和对账基础已经具备较好的工程骨架，但现在仍不适合直接进入双支付渠道、企业钱包或动态 RBAC 并行开发。当前最重要的工作是完整关闭 P0.5 Money Safety Foundation，再以单一支付渠道形成可上线的纵向闭环。

建议采用以下主顺序：

1. 完成 P0.5：账务并发不变量、计费指标、Worker 可观测、告警、干净环境部署和安全门 Harness。
2. 完成支付中立契约与通道路由骨架。
3. 先完成支付宝全链路，包括下单、验签、入账、主动查单、补偿 Worker、沙箱和生产小额验证。
4. 在支付宝链路稳定后复制能力到微信支付，不同时推进两套资金链路。
5. 完成支付账单对账和 epay 退场，再开放公开收费。
6. 进入企业 MVP：企业申请、成员关系、企业共享钱包、网关钱包解析、月度上限。
7. 完成动态 RBAC 和企业控制台。
8. 将 `ai-nuxt` 接入真实公开数据，同时以视觉回归保护现有页面样式。
9. 持续建设推理遥测、安全、压测和故障注入，不等待所有业务功能结束后再补。

任何阶段都不得改变以下资金原则：对账只能发现差异和生成复核项；钱包调整必须由有权限的人工动作触发，并经 Wallet Service 写入不可重复的 Ledger 记录。

## 2. 当前工程事实

| 领域 | 已有能力 | 当前缺口 | 建议结论 |
|---|---|---|---|
| 计费与钱包 | Reserve、Settle、Commit、Release、TopUp、行锁、幂等键、decimal 金额、不变量证据；P0.5 并发矩阵和基础资金指标均已通过 | Worker lease 指标和生产告警尚未完成 | 以 TH-P05-03/04 作为既有证据，完成 TH-P05-11/05 后由 Harness 固化为支付前门禁 |
| B5 治理 | 最大费用预留、统一结算兜底、`undercharged` 证据、L4 对账差异、低基数资金指标已落地 | 生产告警未建立 | 由 TH-P05-05 完成告警闭环，禁止恢复任何自动补扣路径 |
| 备份恢复 | 备份基线、两轮恢复演练和干净环境部署验证均已完成 | 尚未汇总进生产安全 Gate | 将 TH-P05-06/07/08 的实测证据交给 TH-P05-09 |
| 支付 | epay 下单、回调验签、幂等入账、PayURL 持久化、管理员补单 | `Gateway` 无 QueryOrder；工厂仍固定 epay；无官方支付宝/微信；无丢回调补偿 Worker | 先做通用契约，再做支付宝单渠道纵向闭环 |
| 对账 | L0-L4 差异发现，undercharge 路径只读钱包 | 无正式 review item、审核动作和显式 adjustment 流程 | 执行 TH-P5-RVW-*，持续禁止 Worker 直接改钱包 |
| 企业 | tenant/membership/invitation 数据基础、单域名 membership 上下文 | 网关仍普遍调用 `FindByUser(..., nil)`，实际扣个人钱包；企业自服务不完整 | 先完成 holder 设计和企业钱包服务，再逐端点接入网关 |
| RBAC | 平台管理员与企业 owner/admin/member 基础角色 | 平台后台仍是 `user/admin` 二态；前后端缺少细粒度权限目录 | 按 P3 从权限目录、迁移、仓储、中间件到 UI 逐层落地 |
| 控制台 | React/Vite/TypeScript 控制台，测试基础较完整 | 当前 lint 有错误与 Hook 依赖警告 | 在大规模企业 UI/RBAC UI 前建立 lint 零错误基线 |
| 公共站点 | `ai-nuxt` 已是 Nuxt 4 + Vue 3 + TypeScript + Tailwind CSS v4，已纳入本仓库 | 数据仍有本地快照依赖，目录合并不等于真实接口接入 | P4 按同仓多应用交付处理，视觉基线先于数据接入 |
| 可观测性 | 健康检查、`/metrics`、P0.5 低基数资金指标 | Worker lease 指标未完成；无 request/endpoint 级推理事实表 | TH-P05-11 收口 Worker 可观测，P5-TEL 单独建设推理遥测 |

## 3. 当前执行快照

`TASK_INDEX.md` 当前记录 139 个 Sprint Task。P0.5 已完成：

- `TH-P05-01` B5 Reserve Maximum Charge Calculation
- `TH-P05-02` B5 Settle Fallback Visibility Correction
- `TH-P05-03` Billing Invariant And Concurrency Tests
- `TH-P05-04` Basic Gateway Billing Metrics
- `TH-P05-06` Database Backup Baseline
- `TH-P05-07` Backup Restore Drill
- `TH-P05-08` Clean Environment Deployment Verification
- `TH-P05-10` Payment Order PayURL Persistence Fix
- `TH-P05-12` B5 Non-Chat And Multimodal Coverage Closure

三个任务的最新结果：

- `TH-P05-03`：真实 Postgres 钱包与网关并发矩阵全部 PASS，未发现负余额、双扣、双释放、预留前调用 Provider 或账本不一致。
- `TH-P05-04`：Reserve、Settle、Release、pricing incomplete、undercharge 等低基数指标已接入并通过 AC；`/metrics` 可抓取，健康检查行为不变。
- `TH-P05-08`：干净数据库迁移、一次安全 down/up、生产配置 fail-fast、API/Worker/Redis/Postgres、网关计费、重启幂等、对账和资金不变量均已实测 PASS。

执行证据分别位于 `docs/tasks/execution-logs/TH-P05-03.md`、`TH-P05-04.md` 和 `TH-P05-08.md`。后续任务应直接消费这些证据，不重复开发。

P0.5 现在只剩：

- `TH-P05-11` Worker Lease Observability
- `TH-P05-05` Basic Production Alerts
- `TH-P05-09` Production Safety Gate Harness

## 4. P0.5 收口实施方案

### 4.1 推荐执行图

```text
TH-P05-11 -> TH-P05-05 -> TH-P05-09 -> Payment work unlocked

Already complete evidence:
TH-P05-01, 02, 03, 04, 06, 07, 08, 10, 12
```

`TH-P05-11` 应在 `TH-P05-05` 前完成，因为告警 AC 包含 Worker silence，而可靠的 Worker success/skip/error 指标由 `TH-P05-11` 提供。当前 Backlog 只声明 `TH-P05-05` 依赖 `TH-P05-04`，这是需要后续修订的依赖缺口。

### 4.2 TH-P05-03 Billing Invariant And Concurrency Tests（已完成）

实际结果：任务已完成并进入 `TASK_INDEX.md` 的 DONE 状态。该任务保留为单一 Task 是合理的，因为所有场景共同验证同一资金不变量和同一测试夹具，拆开会重复建立 Postgres 钱包、网关请求和并发控制夹具。

开发边界：

- 只新增或调整测试，不借机修改支付 Provider、企业钱包或网关架构。
- 重点文件为 `internal/handler/gateway/*_test.go`、`internal/repository/wallet/*_test.go`。
- 使用真实 Postgres 行锁验证并发，不以纯 mock 替代 AC-01。
- 使用稳定 request id 和 idempotency key，确保失败可以重放定位。

必须形成的证据：

- 两个并发请求竞争不足余额时，最多一个请求进入上游。
- 同一 request id 重复提交时，不产生重复钱包流水。
- 上游失败后，预留被 Release，或按端点规则留下明确失败证据。
- 每个场景结束后均验证 `wallet balance == committed ledger sum`。
- 数据库集成测试必须实际运行；未配置 `TEST_DATABASE_URL` 导致的 skip 不得作为 PASS。

已形成证据：AC-01 至 AC-04 均有测试名和执行结果，`go test`、race 回归、`go vet ./...`、`go build ./...` 均通过，并已生成 `execution-logs/TH-P05-03.md`。

### 4.3 TH-P05-04 Basic Gateway Billing Metrics（已完成）

实际结果：任务已完成并进入 `TASK_INDEX.md` 的 DONE 状态。以下要求作为后续告警、Worker 指标和生产回归的既有基线。

开发边界：

- 建立唯一的 metrics 注册和采集入口，避免各 handler 自建计数器。
- 将 `UnderchargeFallbackCounts()` 的行为接到统一指标；完成后不再把进程内 map 作为生产事实来源。
- 在 Reserve、Settle、Release、pricing incomplete、wallet rejection 的共同边界记录指标。
- 标签只允许 endpoint、request_type、operation、outcome、pricing_state 等有限集合。
- 禁止 user id、request id、order no、email、API key、model prompt 和支付 URL 进入 label。

建议基础指标：

- `tokenhub_billing_operations_total{operation,outcome,endpoint}`
- `tokenhub_billing_operation_duration_seconds{operation,endpoint}`
- `tokenhub_billing_pricing_incomplete_total{endpoint,request_type}`
- `tokenhub_billing_undercharged_total{endpoint}`
- `tokenhub_wallet_rejections_total{reason}`

具体命名可按仓库规范调整，但语义和标签约束必须固定并有测试。

已形成证据：成功请求产生 Reserve+Settle；上游失败产生 Release；pricing incomplete 不产生 Reserve；undercharge 增加一次；`/metrics` 可抓取且健康检查行为不变；标签清洗与高基数字段约束已通过测试。

### 4.4 TH-P05-11 Worker Lease Observability

建议工期：1d。

开发边界：

- 复用 `internal/pkg/lease`，不重写调度器。
- 在现有 health checker、reconciliation、subscriptions Worker 的共同包装层记录 acquired、skipped、failed、panic_recovered 和 duration。
- nil Redis 保持单实例模式；Redis lease 错误保持 fail-closed skip。

建议基础指标：

- `tokenhub_worker_cycles_total{worker,outcome}`
- `tokenhub_worker_cycle_duration_seconds{worker}`
- `tokenhub_worker_lease_total{worker,outcome}`

必须形成的证据：双 Worker 竞争时一方 acquired、一方 skipped；Redis 错误时不执行工作函数；工作函数 panic 后进程仍存活且指标增加。

### 4.5 TH-P05-05 Basic Production Alerts

建议工期：1d。

开发边界：

- 基于 TH-P05-04 和 TH-P05-11 的已落地指标写规则，不依赖日志文本匹配。
- 至少覆盖 undercharge、critical reconciliation diff、worker silence、数据库不可用、Redis 不可用。
- 每条规则包含明确窗口、阈值、严重级别、runbook 链接和恢复条件。
- 在 fixture 中验证 firing 和 not firing，避免上线后才发现表达式无效。

发布阻断建议：任何 undercharge 持续发生、critical reconciliation diff 未关闭、计费 Worker 超时静默、数据库不可用均阻断新支付渠道灰度。

### 4.6 TH-P05-08 Clean Environment Deployment Verification（已完成）

实际结果：任务已完成并进入 `TASK_INDEX.md` 的 DONE 状态。该任务不拆成纯脚本和纯报告是合理的，因为验收对象是同一个不可分割的“从零部署并验证”过程。

开发边界：

- 使用全新容器卷或一次性主机，不复用开发机已有 DB、Redis、构建缓存或 `.env`。
- 固定 commit、镜像 digest、migration 版本和配置模板版本。
- 仅使用 fake/sandbox 上游，不接真实支付凭据。
- 验证 API、Worker、Postgres、Redis、网关调用、usage/billing evidence、reconciliation run。
- 在一次性数据库执行安全的 migration down/up；不得在生产或共享 staging 数据库执行 down。

已形成证据：`/readyz` 返回 200；观察到 Worker lease cycle；网关请求产生 usage、charge lines、provider evidence 和钱包流水；重启重放不重复扣款；对账 run row 被创建；缺少必需密钥或使用弱生产配置时启动失败；隔离库 migration down/up 通过。

### 4.7 TH-P05-09 Production Safety Gate Harness

建议工期：1d 至 1.5d。

开发边界：

- Harness 只聚合已有证据，不重复实现业务 E2E。
- 每项检查输出 check id、PASS/FAIL、duration、evidence path 和 blocker code。
- 缺少 B5、B7、B8、PayURL、指标、告警或 Worker lease 证据时必须非零退出。
- Harness 不要求支付宝、微信或其他生产 Provider 凭据。

P0.5 最终 Gate 必须验证：

| Gate Check | 来源 |
|---|---|
| B5 最大预留与兜底 | TH-P05-01、02、12 |
| 账务不变量与并发 | TH-P05-03 |
| 资金指标 | TH-P05-04 |
| 生产告警 | TH-P05-05 |
| 备份与恢复 | TH-P05-06、07 |
| 干净环境部署 | TH-P05-08 |
| PayURL 回归 | TH-P05-10 |
| Worker lease 可观测 | TH-P05-11 |

只有 Harness 在干净环境中退出 0，P1 支付任务才可开始。

## 5. 支付阶段实施建议

### 5.1 先完成 Provider 中立骨架

执行顺序：

```text
TH-P1-01 QueryOrder contract
  +-> TH-P1-02 settlement intent
  +-> TH-P1-03 channel factory
        +-> TH-P1-04 callback channel resolver
        +-> TH-P1-05 provider metadata
```

建议代码边界：

- `internal/service/payment/types.go`：Provider 中立类型和窄接口。
- `internal/service/payment/service.go`：支付编排和结算意图，不包含具体 SDK 细节。
- `internal/service/payment/<provider>.go`：签名、下单、查单等 Provider 细节。
- `internal/repository/paymentorder/`：订单通道、provider trade id、重试元数据和状态 CAS。
- `internal/worker/`：扫描、调度、重试和补偿，不直接拼 Provider 请求。

接口建议：将 CreateOrder、Notify Verification、QueryOrder 保持为可独立测试的窄能力，再由 channel factory 组合为 Provider adapter。这样 epay 在过渡期没有完整 QueryOrder 时，不会迫使核心接口返回伪结果。

### 5.2 支付宝作为第一个纵向闭环

推荐顺序：

1. `TH-P1-AL-01` 配置与启动校验。
2. `TH-P1-AL-02` CreateOrder。
3. `TH-P1-AL-03` Notify 验签。
4. `TH-P1-AL-04` Notify 入账。
5. `TH-P1-AL-05` QueryOrder。
6. `TH-P1-AL-06` 沙箱集成测试。
7. `TH-P1-AL-07` 生产小额验证 Runbook。

每个任务建议 0.5d 至 1.5d。Provider 私钥、应用证书、平台证书和回调原文不得写日志、指标、测试快照或执行日志。回调入账和主动查单入账必须复用同一订单状态迁移与钱包幂等路径，不允许形成两套资金实现。

支付宝生产小额验证通过前，支付方式只对内部白名单开放。验证至少包含正常支付、重复回调、回调丢失后主动查单、签名失败、金额不一致、Provider 超时和本地 DB 失败。

### 5.3 补偿 Worker

推荐顺序：

```text
TH-P1-CW-01 scanner
  -> TH-P1-CW-02 dispatcher
      +-> TH-P1-CW-03 paid compensation
      +-> TH-P1-CW-04 closed/expired handling
      +-> TH-P1-CW-05 retry/backoff
            +-> TH-P1-CW-06 manual review
      +-> TH-P1-CW-07 lease/idempotency
      +-> TH-P1-CW-08 metrics
  -> TH-P1-CW-09 failure/restart integration
```

关键实现要求：

- Scanner 只读并返回候选订单，不改变钱包和订单状态。
- Dispatcher 必须按订单持久化 channel 路由，不读取当前全局 channel 决定历史订单去向。
- Paid compensation 复用 payment service，使用 `order_no` 幂等键和订单 CAS。
- Amount mismatch、unknown state、unsupported channel、max retry 只生成 review item，不入账也不扣款。
- Redis lease 只减少重复运行，正确性仍由订单 CAS 与钱包幂等共同保证。
- Worker 重启后从持久化 retry metadata 恢复，不依赖内存计数。

当前 `TH-P1-CW-02` 同时依赖 `TH-P1-AL-05` 和 `TH-P1-WX-05`，会迫使支付宝补偿链路等待微信 QueryOrder。建议在开始 P1 前修订为“Dispatcher core + provider registration”，或允许该任务先依赖一个已启用 Provider 的 QueryOrder。修订前不要绕过依赖直接执行。

### 5.4 微信支付与 epay 退场

微信任务 `TH-P1-WX-01` 至 `TH-P1-WX-07` 应在支付宝沙箱和补偿链路稳定后执行。其状态机、结算意图、订单 CAS、Worker、指标和 review item 应全部复用已有中立能力，只新增微信特有的证书、签名、解密和 API 客户端。

epay 退场必须分两步：先禁止创建新订单，再保留历史 pending 订单的回调窗口。不得因切换默认渠道而删除旧回调路由、旧订单、支付链接或审计证据。

`TH-P1-RC-01` 当前同时依赖支付宝和微信沙箱测试，也会阻塞单渠道先上线。建议将“账单导入契约”保持 Provider 中立，再为每个 Provider 建立独立 importer 验证；最终 cutover 才要求双渠道证据齐备。

## 6. 企业阶段实施建议

企业能力应分为四个资金和权限边界，不能按“企业模块”一次性开发。

### 6.1 企业申请与成员关系

执行 `TH-P2-01` 至 `TH-P2-06`、`TH-P2-INV-01` 至 `TH-P2-INV-06`。

开发重点：

- 企业申请校验与持久化使用事务，重复申请有确定错误码。
- 审核动作记录 before/after、operator、reason 和 request id。
- 邀请 token 只保存哈希，接受邀请必须幂等。
- owner 自锁死保护必须覆盖降级、移除、最后 owner 等边界。
- suspended/rejected 企业不得获得企业钱包和企业网关上下文。

### 6.2 企业共享钱包

执行 `TH-P2-WAL-01` 至 `TH-P2-WAL-09`。

建议坚持独立 `enterprise_wallets` / enterprise ledger 边界，不在现有个人钱包表中通过 nullable tenant 字段继续扩展语义。Repository、Reserve、Settle、Release、TopUp、Listing 和不变量测试按任务独立评审。

企业钱包的每个资金动作必须满足：decimal 金额、行锁或 CAS、幂等键、不可变流水、余额不为负、失败事务不产生部分状态、回滚迁移经过隔离库验证。

### 6.3 网关钱包解析

执行 `TH-P2-GW-01` 至 `TH-P2-GW-07`。

当前 `chat.go`、`endpoints.go`、`claude_relay.go`、`video.go`、`responses_via_chat.go` 等路径仍直接调用 `FindByUser(..., nil)`。必须先建立唯一 Wallet Holder Resolver，再逐类端点接入。禁止每个 handler 各自判断 tenant membership，否则会出现同一企业请求在不同端点扣不同钱包。

迁移顺序：Chat -> Responses/Claude -> Embeddings/Images -> Audio/Video -> 全端点 Settle/Release 矩阵。每一步都要保留个人用户回归和 suspended enterprise fail-closed 测试。

### 6.4 成员月度上限

执行 `TH-P2-LIM-01` 至 `TH-P2-LIM-07`。

实现顺序必须是 schema -> admin API -> Redis counter -> DB fallback -> fail-closed -> concurrency -> final charge correction。额度必须以 Reserve 阶段占用，Settle 后按实际费用校正；不能只在请求完成后累计，否则并发请求可以共同穿透上限。

## 7. RBAC 实施建议

执行顺序：

```text
Permission catalog
  -> migration/seed
  -> role repository
  -> role APIs and assignment guard
  -> current-user permissions
  -> RequirePerm middleware
  -> route permission matrix and audit
  -> frontend hydration/navigation/action gating/UI
```

后端权限判断是唯一安全边界，前端隐藏按钮只负责用户体验。每一组后台路由从 `AdminAuth()` 迁移到 `RequirePerm()` 时，必须覆盖未登录、无权限、有权限、Super Admin 和自锁死保护。迁移应按路由组逐步完成，并保留可回滚到旧二态管理员守卫的路径。

权限目录应是固定代码常量；角色是权限集合；Super Admin 是受保护的系统角色。禁止允许管理员删除最后一个 Super Admin、移除自己的关键权限或创建未知 permission key。

## 8. 公共站点与控制台建议

### 8.1 仓库责任边界

- `DeepTrolsTokenHub/web`：React 控制台和管理后台。
- `/Users/olly/Documents/项目/智曜TokenHub/ai-nuxt`：Nuxt 公共首页和模型商店。
- `DeepTrolsTokenHub`：公开 API、认证 API、支付、网关、企业和管理 API。

P4 文档中涉及 `ai-nuxt/server/api/*.get.ts` 的任务现在在本仓库执行。每个前后端联调任务必须在执行日志记录同时覆盖后端与 Nuxt 的提交、接口契约版本和联调环境，避免只提交一侧后无法复现。

### 8.2 Nuxt 页面原则

- 保持现有视觉，不把数据接入变成页面重设计。
- 使用 Nuxt 4、Vue 3、TypeScript、Composition API、`<script setup>`、Tailwind CSS v4 theme bridge 和现有 SCSS。
- 组件模板优先 Tailwind utility，不新增无必要的自建选择器。
- 真实数据接入前先完成桌面和移动端截图基线。
- 模型卡片、模型图标、弹层、Header、登录区和内容宽度均纳入视觉回归。
- Nitro 代理负责服务端地址、超时、缓存和 fallback metadata，浏览器不直接依赖内部后端地址。

### 8.3 React 控制台质量门槛

在 P2 企业 UI 或 P3 RBAC UI 开始前，建议建立一个独立的小型质量任务，关闭当前 ESLint 的 4 个 error 和 4 个 warning，并将 lint 纳入 CI 必过项。不得把 lint 修复混进企业钱包或 RBAC 资金/权限任务。

## 9. 对账、调整与推理遥测

### 9.1 Reconciliation Review

执行 `TH-P5-RVW-01` 至 `TH-P5-RVW-07`，固定流程如下：

```text
Reconciliation
  -> reconciliation_diff
  -> review_item
  -> human decision
       +-> close/waive
       +-> explicit adjustment command
             -> Wallet Service
             -> immutable Ledger
```

禁止任何 reconciliation 或 worker 代码调用 Wallet Spend、Adjust、TopUp 来自动处理 undercharge。Adjustment 必须记录 operator、reason、source_diff_id、before_balance、amount、after_balance；相同 source_diff_id 只能成功一次；原始 Usage、Ledger 和 Evidence 不可覆盖修改。

### 9.2 Inference Telemetry

`TH-P5-TEL-01` 至 `TH-P5-TEL-09` 应在 P0.5 通过后尽早开始，可与 Provider 开发并行，但不能阻塞首个支付闭环。

事实表至少记录：model、provider、channel、endpoint、request_type、latency、ttft、duration、input_tokens、output_tokens、tokens_per_second、status、error_type、retry_count、provider_cost、customer_cost、gross_margin。

遥测写入失败不得影响主请求和资金结算；可采用有界缓冲和失败指标，但必须明确丢弃策略。Prometheus 用于聚合运行指标，Telemetry persistence 用于 request/endpoint 级历史分析，两者必须分开设计。

## 10. 推荐迭代安排

以下以两周 Sprint 为参考。团队人数变化时可以调整每个 Sprint 的任务数量，但不得改变依赖顺序和 Gate。

### Sprint 1：Money Safety Foundation

目标：P0.5 Harness 在干净环境退出 0。

剩余范围：`TH-P05-11`、`TH-P05-05`、`TH-P05-09`。`TH-P05-03`、`TH-P05-04`、`TH-P05-08` 与其他已完成任务作为证据输入，不重复开发。

不包含：支付宝、微信、企业、RBAC、公共页面功能。

### Sprint 2：Payment Core + Alipay Entry

目标：建立 Provider 中立骨架，并完成支付宝配置、下单和回调验签，不进行公开收费。

建议范围：`TH-P1-01` 至 `TH-P1-05`、`TH-P1-AL-01`、`TH-P1-AL-02`、`TH-P1-AL-03`。

Sprint 前置动作：修订 P1 的单渠道依赖问题，并为 P0.5/P1 任务补齐 0.5d 至 2d Estimate。

### Sprint 3：Alipay Money Path + Query

目标：支付宝回调入账、主动查单和沙箱链路全部通过。

建议范围：`TH-P1-AL-04`、`TH-P1-AL-05`、`TH-P1-AL-06`、`TH-P1-CW-01`、单渠道可执行的 `TH-P1-CW-02`、`TH-P1-CW-03`、`TH-P1-CW-04`。

### Sprint 4：Alipay Recovery + Controlled Production

目标：补偿 Worker 可重试、可重启、可观测，并完成支付宝生产小额验证。

建议范围：`TH-P1-CW-05` 至 `TH-P1-CW-09`、`TH-P1-AL-07`、支付宝账单导入的首个 Provider 实现。

### Sprint 5：WeChat Vertical Slice

目标：复用支付中立能力完成微信下单、回调、查单、沙箱和生产小额验证。

建议范围：`TH-P1-WX-01` 至 `TH-P1-WX-07`，并扩展 Dispatcher 和 Worker 集成测试覆盖微信。

### Sprint 6：Payment Reconciliation + Epay Cutover

目标：双官方渠道具备账单对账和历史订单处理能力，epay 停止创建新订单。

建议范围：`TH-P1-RC-01` 至 `TH-P1-RC-06`、必要的 `TH-P5-RVW-01` 至 `TH-P5-RVW-03`。

### Sprint 7：Enterprise Identity + Wallet Foundation

目标：企业申请、审核、邀请、成员治理和企业钱包基础完成，但网关尚不切换企业扣费。

建议范围：`TH-P2-01` 至 `TH-P2-06`、`TH-P2-INV-*`、`TH-P2-WAL-01` 至 `TH-P2-WAL-05`。

### Sprint 8：Enterprise Charging + Limits

目标：企业请求从共享钱包 Reserve/Settle/Release，成员月度上限并发不穿透。

建议范围：`TH-P2-WAL-06` 至 `TH-P2-WAL-09`、`TH-P2-GW-*`、`TH-P2-LIM-*`。

### Sprint 9：Enterprise Console + RBAC Backend

目标：企业管理闭环可用，平台后台开始由权限目录控制。

建议范围：`TH-P2-UI-*`、`TH-P3-01` 至 `TH-P3-11`。

### Sprint 10：RBAC Frontend + Public Site Data

目标：控制台权限显示与后端一致，Nuxt 首页和模型商店读取真实公开数据且视觉不变。

建议范围：`TH-P3-12` 至 `TH-P3-15`、`TH-P4-01` 至 `TH-P4-12`。

### 持续并行轨道：Production Hardening

在 P0.5 通过后，根据主线能力逐步执行 `TH-P5-RVW-*`、`TH-P5-TEL-*`、`TH-P5-MET-*`、`TH-P5-SEC-*`、`TH-P5-LOAD-01`。不要把全部 P5 推迟到最后一个 Sprint。

## 11. 发布 Gate

| Gate | 允许动作 | 必须证据 |
|---|---|---|
| G0 P0.5 Safety | 开始 Provider 开发 | TH-P05-09 报告为 PASS |
| G1 Alipay Sandbox | 开启内部测试充值 | 下单、验签、回调、QueryOrder、重复请求、金额不一致均通过 |
| G2 Alipay Controlled Production | 小额白名单真实支付 | TH-P1-AL-07 报告、补偿 Worker、指标告警、回滚方案 |
| G3 Official Payment Public | 对外开放收费 | 支付对账、历史订单窗口、epay cutover、无 P0/P1 缺陷 |
| G4 Enterprise Pilot | 开放企业试点 | 企业钱包不变量、网关全端点矩阵、月度上限并发测试 |
| G5 General Availability | 扩大客户范围 | RBAC、审计、Telemetry、安全轮换、负载与故障注入 |

## 12. 测试和证据要求

每个 Task 应按风险选择以下证据，并写入独立 execution log：

- Unit：状态映射、金额校验、标签清洗、权限矩阵、重试计算。
- Integration：真实 Postgres 事务/行锁、Redis lease、migration up/down、Repository CAS。
- Contract：Provider request/response、签名、证书、回调和 QueryOrder 状态归一。
- Concurrency：余额竞争、重复回调、双 Worker、月度额度竞争、重复 adjustment。
- Failure Injection：Provider 超时、Redis 错误、DB 错误、Worker panic/restart、部分失败。
- E2E：支付沙箱、企业角色矩阵、控制台权限、Nuxt 真实数据和弹层行为。
- Visual Regression：公共首页、模型商店、模型弹层、Header、登录区、桌面和移动端宽度。
- Operations：备份、恢复、干净部署、密钥轮换、生产小额验证、回滚演练。

数据库集成测试被环境变量跳过时，任务只能记录为未验证，不能标记 DONE。任何资金 Task 的 execution log 必须明确写出：负余额是否可能、Provider 是否可能在 Reserve 前被调用、Ledger 是否一致，以及哪些场景未测试。

## 13. Backlog 治理建议

在下一次只改文档的 Backlog 维护轮次中，建议处理以下事项：

1. 为 P0.5 和 P1 的全部 Sprint Task 补齐 0.5d 至 2d Estimate；当前 `TASK_INDEX.md` 使用 `—`，无法进行容量规划。
2. 将 `TH-P05-05` 的依赖补充 `TH-P05-11`，使 Worker silence 告警具备真实指标来源。
3. 复核 `TH-P05-09` 是否应显式依赖 `TH-P05-12`，避免安全 Gate 仅依赖间接/历史证据。
4. 调整 `TH-P1-CW-02` 对支付宝和微信 QueryOrder 的双重依赖，使补偿 Worker 可以随首个 Provider 形成纵向闭环。
5. 调整 `TH-P1-RC-01` 的双 Provider 前置，区分中立账单契约与 Provider importer。
6. 修正 `BACKLOG_OVERVIEW.md` 开头“138 个”与统计表“139 个”的不一致。
7. 修正 `PRODUCTION_READINESS.md` 中“对账按 undercharged 标记自动补收”的旧表述。正确策略是生成 diff/review item，人工审核后显式 adjustment。
8. P4 任务以本仓库 `ai-nuxt/` 为工作路径，保留前后端变更提交、契约版本和联调证据要求。
9. 为 React 控制台 lint 基线和前后端契约验证建立独立小任务，不混入企业资金或 RBAC Task。

上述建议本文件仅记录，不在本次直接修改现有任务定义或依赖。

## 14. 交付纪律

- 一个 Sprint Task 对应一个可独立评审、测试、验收和回滚的变更单元。
- 一个 PR 原则上只完成一个 Task；共享底层变更先落独立 Task，再由后续任务依赖。
- Migration Task 必须包含 up/down、干净库、带数据升级和回滚证据。
- 资金、权限、Provider Task 不与无关重构、格式化或 UI 调整混在同一 PR。
- Task 完成后只更新 `TASK_INDEX.md` 对应状态并生成独立 execution log。
- 发现 Backlog Gap 时先记录，不在当前 Task 扩大 Scope。
- 任何日志、指标、报告和截图都不得包含密钥、Token、支付 URL、签名原文或支付敏感信息。

## 15. 下一步

当前最合适的动作不是启动新的业务 Epic，而是按以下顺序收口当前 Sprint：

1. 将已完成的 `TH-P05-03`、`TH-P05-04`、`TH-P05-08` 固定为 P0.5 Harness 输入证据，不重复开发。
2. 基于 TH-P05-04 完成 `TH-P05-11`。
3. 基于资金指标和 Worker 指标完成 `TH-P05-05`。
4. 汇总全部 P0.5 证据完成 `TH-P05-09`。
5. 单独进行一次 Backlog 文档修订，处理第 13 节依赖和估算问题。
6. Gate 通过后启动 Sprint 2，仅进入 Payment Core 和支付宝入口，不同时启动微信、企业或 RBAC。

判断标准很简单：在 `TH-P05-09` 真实通过前，项目仍处于“安全基线建设期”；通过后才进入“真实支付交付期”。

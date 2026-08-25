# 国内企业级 Token 网关实施计划（借鉴 TokenHub）

> 目标：把 DeepTrols 从"计费平台"升级为"国内模型企业级 Token 治理网关"，借鉴 TokenHub
> 的治理 / 路由 / 适配器三支柱，但**明确不做国外模型**（OpenAI / Anthropic / Gemini /
> Codex 原生协议、OAuth 订阅、国际 Provider 模板）。
>
> 参考项目：`G:/workspace/demo/TokenHub-main`（Apache-2.0）。
> **许可证策略（2026-08-25 已确认）：直接拷贝**——可 fork 的模块整包复制，保留 Apache-2.0
> 头部与 NOTICE；拷贝内容与本仓库其余代码隔离目录放置并标注来源。
> 制定日期：2026-08-25。

## 目标

1. **治理强**：调用前准入 = Key 状态/权限 + RPM/TPM 分钟桶 + 项目/团队预算 + 审批 + 出站内容策略（Guardrails，国内合规刚需）。
2. **路由强**：model_routes 有序候选 + 优先级/权重分组 + 策略（priority_only / cost / quality）+ 粘性会话 + 一致性哈希 + 缓存亲和 + 资源健康/冷却/并发租约。
3. **适配器全**：一个 OpenAI 兼容核心适配器 + 15 家国内 Provider 模板 + 能力注册表 + usage/响应归一化（cache / reasoning / image / tts / video）。
4. **保留护城河**：钱包 Reserve→Settle→Release、decimal、价格快照、三表证据链、L0/L1 对账、租户/OEM 隔离、流式失败证据。
5. **前端补齐**：纯 TS 领域层 + 治理页面（审计/审批/Guardrails/限流策略）+ 路由模拟器与网关健康视图 + Provider 编辑器升级 + Playwright E2E。

## 范围

### 做

- 国内 Provider 模板：DeepSeek、Qwen、智谱、Kimi、豆包、混元、讯飞、文心、零一、SiliconFlow（OpenRouter 可选，仅聚合国内）。
- 端点：`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`、`/v1/images/generations`、`/v1/audio/speech`（已有）；`/v1/audio/transcriptions`、`/v1/videos/generations`（豆包 Seedance / 可灵，异步任务）、`/v1/images/edits`（按需）。
- 治理：Key RPM/TPM 分钟桶、项目/团队预算 + 审批、Guardrails 内容策略引擎。
- 路由：策略排序、粘性会话、一致性哈希、缓存亲和、资源冷却、并发租约。
- 前端：领域逻辑分层（`web/src/lib/domain/` + 单测）、治理页面、路由模拟器、Provider 编辑器、E2E。

### 不做

- `/v1/messages`（Anthropic 协议）、`/v1/responses`（OpenAI 新协议）、Gemini 原生、Codex 桥接——国外模型专属。
- OpenAI / Anthropic / Gemini / Codex 的 Provider 模板与 OAuth 订阅账户。
- 150+ 国际 Provider 模板全量接入（仅保留国内子集）。
- 已移除的旧"配额池"（quota_pools/allocations/ledger）**不恢复**；治理按本计划的新模型（分钟桶 + 预算）重建。

## 现状（已核实）

- 路由数据模型：`models → channels → channel_instances`（`migrations/000001_init.up.sql`），渠道有 `pool_type / health_score / health_status / weight / max_concurrency`，实例有 `current_load / max_load / config`。
- 路由逻辑：`internal/service/gateway/router.go` — 租户选品过滤 → 渠道按 weight/(max_concurrency+1) 评分 → 实例按 Redis 实时负载（`loadtracker.go`）选择 → 候选 failover（`chat.go` candidates 循环）。
- 执行：单一 OpenAI 兼容直连（`internal/service/gateway/executor.go`，类名遗留 `LiteLLMExecutor`）；raw 端点转发（`endpoints.go`：embeddings / images / audio/speech）。
- 计费：`internal/service/billing/` — pricer（成本/售价分离 + 峰谷 + PAYG 门禁）、charger（钱包 Reserve/Settle/Release）、logger（三表事务 + 价格快照）。
- 准入现状：API Key 六边界（`internal/handler/gateway/boundaries.go`）+ 钱包余额；无 RPM/TPM 桶、无预算、无 Guardrails。
- Provider 模板：`internal/handler/console/providers.go` `defaultBaseURLs` 14 家（含国外），前端 `web/src/pages/Channels.tsx` `PROVIDER_OPTIONS` 15 项。
- 响应缓存已有（`internal/service/cache` + `chat.go` X-Cache），可支撑缓存亲和。
- 健康检查：`internal/worker/health_checker`（±30 渐进评分），无 cooldown 窗口。
- 迁移链：`migrations/000001..000014`；测试基建每包独立 schema（`internal/repository/testutil`）。

## 目标架构

```
调用前准入（治理）
  Key 状态/权限（已有六边界 + 租户隔离）
  → RPM/TPM 分钟桶（Redis + DB 双写，回滚）
  → 项目/团队预算（预算账本 + 审批）
  → Guardrails 内容策略（确定性匹配器 + 工作量预算）
        ↓
路由（强路由）
  model_routes（模型 → 渠道资源，有序候选）
  → 优先级/资源优先级/权重分组
  → 组内策略：priority_only / cost / quality
  → 粘性会话 / 一致性哈希 / 缓存亲和
  → 资源健康 / cooldown / 并发租约
        ↓
执行（适配器全）
  AdapterRegistry（provider 类型 → 适配器 + 能力声明）
  OpenAI 兼容核心适配器 + 国内 Provider 模板
  usage/响应归一化（cache / reasoning / image / tts / video）
        ↓
资金面（保留）
  钱包 Reserve→Settle→Release + 价格快照 + 三表证据链 + 对账 L0/L1
```

## 分期实施

### Phase 0：数据模型演进（1–2 天，核心路径，TDD）

**目标**：让 `channels / channel_instances` 具备"资源健康/冷却/并发租约/策略"语义，不动计费表。

1. 迁移 `000015_gateway_resources.up/down.sql`：
   - `channel_instances` 增加 `cooldown_until TIMESTAMPTZ`、`last_checked_at TIMESTAMPTZ`、`concurrency_limit INT NOT NULL DEFAULT 10`（现 `max_load` 保留兼容）。
   - `channels` 增加 `strategy VARCHAR(16) NOT NULL DEFAULT 'priority_only'`、`sticky_session BOOLEAN NOT NULL DEFAULT FALSE`、`fallback_order INT NOT NULL DEFAULT 0`。
   - 可选：`model_routes` 物化视图或查询封装（模型 → 有序渠道候选），先以仓储查询实现，不建冗余表。
2. `internal/domain/channel.go`：`Channel` / `ChannelInstance` 增加字段；`Strategy` 常量（`priority_only` / `cost` / `quality`）。
3. `internal/repository/channel/postgres.go`：`ListByModel` 增加策略/粘性/冷却字段扫描；新增 `EnterCooldown`、`ClearCooldown`。
4. 测试：repository 层（字段读写 + cooldown 过滤）、domain 纯逻辑（策略常量校验）。

### Phase 1：治理层（2–3 天，核心路径，TDD）

**目标**：调用前准入补齐 RPM/TPM、预算、审批、Guardrails。

1. **RPM/TPM 分钟桶**（参考 TokenHub `store_call_admission.go` `consumeAPIKeyMinuteRequest`）：
   - `internal/pkg/ratelimit` 扩展或新增 `internal/pkg/quota_bucket`：Redis hash + Lua（分钟桶、原子自增），Redis 不可用降级 DB 表 `api_key_quota_buckets`（`migrations/000016`）。
   - `internal/domain/apikey.go`：`APIKey` 增加 `rate_limit_rpm / rate_limit_tpm`。
   - 网关 `internal/handler/gateway/boundaries.go`：准入时按估算 token 预留扣桶，超限 429 + `X-RateLimit-*` 头；请求结束按真实 usage 回填。
2. **项目/团队预算**：
   - `migrations/000016`：`budgets`（tenant/项目维度：period、limit、spent、status）+ `budget_requests`（申请/审批）。
   - `internal/service/billing`：`BudgetChecker`（预留 → 结算 → 回滚，复用钱包幂等模式）。
   - Console/Admin API：企业主查看预算、申请加额；平台管理员审批（`/api/admin/budgets/requests/{id}`）。
3. **Guardrails 内容策略**：
   - 新包 `internal/guardrails`（参考 TokenHub `internal/guardrails/engine.go`）：确定性匹配器 + 正则（带工作量预算防 ReDoS）+ 绑定（项目/租户维度），动作 allow/block。
   - 网关 chat 路径在路由前评估；命中 block 返回 400 `guardrail_blocked` 并落审计。
4. 测试：分钟桶并发/回滚、预算预留/结算、Guardrails 匹配器 + 工作量上限。

### Phase 2：强路由（2–3 天，核心路径，TDD）

**目标**：路由从"权重/负载评分"升级为"策略化排序 + 亲和 + 冷却"。

1. `internal/service/gateway/router.go`：
   - 按 `priority → resource priority → weight` 分组；组内按 `strategy` 排序：
     - `priority_only`：现行为（评分排序）；
     - `cost`：按 `internal/service/billing` pricer 单价升序；
     - `quality`：按健康分数降序（复用 `health_checker` 的 ±30 逻辑）。
   - 粘性会话：`sticky_session=true` 的渠道组内优先（按 user+model 哈希稳定选中）。
   - 一致性哈希：请求级 `routingKey`（request_id 或亲和 key）对组内候选做 rendezvous 加权排序（参考 TokenHub `gateway_http.go planRouteOrder`）。
   - 缓存亲和：命中已有响应缓存（`internal/service/cache`）时优先同缓存域渠道。
2. `internal/handler/gateway/chat.go` / `endpoints.go`：
   - 候选执行前 `CheckProviderResourceCapacity`（并发租约，Redis INCR/DECR，参考现有 LoadTracker 模式）；超限渠道跳过并记录 attempt。
   - 上游失败 → `EnterCooldown(instance, cooldown_until)`；健康检查恢复后 `ClearCooldown`。
3. 测试：策略排序表驱动、粘性/哈希确定性、冷却跳过、并发租约不超限。

### Phase 3：适配器全（3–5 天，非核心但工程量大，TDD 覆盖解析）

**目标**：一套 OpenAI 兼容核心 + 15 家国内 Provider 模板 + usage 归一化。

1. 新包 `internal/provider`：
   - `registry.go`：`ProviderRegistry`（类型 → 适配器 + 能力声明：chat / embeddings / images / audio / tts / video / probe），参考 TokenHub `adapter_registry.go`。
   - `openai_compat.go`：核心适配器（现 `internal/service/gateway/executor.go` 迁移进来，保留直连逻辑）。
2. Provider 模板：
   - `internal/provider/templates.go`：15 家国内模板（base_url、鉴权头、模型列表、usage 差异）。
   - 替换 `internal/handler/console/providers.go` `defaultBaseURLs` 与前端 `Channels.tsx` `PROVIDER_OPTIONS`：去掉 openai / anthropic / google / openrouter 国外项（openrouter 可选保留）。
3. usage 归一化：
   - `internal/pkg/usageparser` 扩展：DeepSeek/Qwen 的 `prompt_cache_hit_tokens`、reasoning token、图片 n、TTS 字符——映射到 9 维定价。
   - 响应转换层：`internal/provider/normalize.go`（上游响应 → 标准 usage + 计费维度）。
4. 能力探测：Provider Sync 时探测 chat/embedding/image/audio 能力并写入 models.category（现有 `discoverModels` 扩展）。
5. 测试：每家模板的请求/响应/usage 解析表驱动；registry 能力解析。

### Phase 4：端点补全（按需，2–4 天，核心路径，TDD）

1. `/v1/audio/transcriptions`：multipart 解析 → raw 转发 → TTS/STT 字符计费（复用 `endpoints.go` raw 模式）。
2. `/v1/videos/generations`（豆包 Seedance / 可灵）：
   - `migrations/000017`：`async_jobs`（id、user、key、model、status、result_url、created_at）。
   - 端点：POST 创建任务、GET 查询、GET content 下载、DELETE 取消（异步轮询/回调）。
   - 计费：按任务创建预留、完成结算（沿用 Reserve→Settle）。
3. `/v1/images/edits`（按需）。
4. 明确不实现国外协议端点（见"不做"）。

### Phase 5：前端补齐（2–3 周，跨批次推进，测试先行）

**目标**：前端从"页面内联逻辑"升级为"领域层 + 治理页面 + 工程化"，对齐 TokenHub 前端的
结构性差距，同时保留现有用户门户/管理后台两套布局。

1. **纯 TS 领域层（治本）**：
   - 新建 `web/src/lib/domain/`：从组件中抽出过滤器（API key / 模型）、金额与日期格式化、
     角色可见性判断、表单默认值、CSV 导入解析等纯函数；组件只保留渲染/交互。
   - 单测：每个模块配 `*.test.ts`（Vitest），对齐 TokenHub `features/admin/domain/*.test.mjs`
     的做法（先测试后实现，前端 TDD）。
   - 权限裁剪：`canAccessView` / `defaultViewForRole` 纯函数（对齐 TokenHub `core/navigation`），
     管理端导航按角色过滤。
2. **治理页面（配合 Phase 1/2 后端）**：
   - 审计日志页：后端 `audit_logs` 已有数据，补 `/api/admin/audit` 列表接口 + 页面。
   - 预算与审批页：企业主申请加额、平台管理员审批流（配合 Phase 1 `budget_requests`）。
   - Guardrails 策略编辑器：策略 CRUD + 绑定项目/租户 + 检测项配置（配合 Phase 1）。
   - 限流策略页：Key 级 RPM/TPM 配置（配合 Phase 1 分钟桶）。
3. **路由与网关可视化（配合 Phase 2）**：
   - 路由策略模拟器：选模型/场景 → 预览候选排序与 failover 顺序（对齐 TokenHub
     `routing-policy-simulator`）。
   - 网关健康视图：渠道/实例健康、实时负载、冷却状态一览。
4. **Provider 编辑器升级**：多 section 表单（基础 / 鉴权 / 能力探测 / 自定义请求头 / 模型映射），
   替换现有单表单与假"测试"按钮（接入真实连通性探测，配合 Phase 3 能力注册表）。
5. **接口文档页**：可选接入后端 OpenAPI 生成（swagger-ui），或结构化补齐现有 Docs 页。
6. **工程化**：
   - Playwright E2E：登录 → 建渠道 → 模型目录 → 网关调用一次 → 账单/审计可见的核心链路冒烟。
   - 可选 i18n（中/英）与暗色主题（国内场景按需）。

> 前端验证：`npm test`（领域单测 + 组件测试）、`npm run lint`、`npm run build`（tsc -b +
> vite build）、Playwright E2E；涉及新后端接口时按核心路径补 Go 测试。

## 借鉴资产（Apache-2.0，带署名可复用）

- 分钟桶配额（Redis + DB 双写、回滚、`X-RateLimit-*` 头）。
- rendezvous 一致性哈希 / 缓存亲和排序（`gateway_http.go planRouteOrder`）。
- Guardrails 确定性匹配引擎（含 ReDoS 工作量预算）。
- Provider Resource 健康 / cooldown / 并发租约模型。
- 150+ Provider 模板中的国内子集。

> 复用注意：拷贝代码需保留 Apache-2.0 头部与 NOTICE；本项目 LICENSE 若仍为内部私有，
> 仅借鉴实现思路并标注来源，不整体搬入闭源。

## 测试与验证策略（AGENTS.md）

- **核心路径（治理/路由/网关/计费）**：先写失败测试（RED）→ 实现（GREEN）→ 重构；覆盖 ≥80% 目标。
- **非核心（模板/文档/迁移）**：轻量，改完跑全量验证。
- **每次变更完成前对现实验证**：`go test ./... -count=1`（含 TEST_DATABASE_URL 真实 PG）、
  `go vet ./...`、`go build ./...`、gofmt、前端 `npm test` + `lint` + `build`。
- 新增迁移必须验证 up/down 往返 + 旧库升级路径（`migrate up` 到最新）。
- 审查：核心路径过 code-reviewer；鉴权/计费/用户输入过 security-reviewer。

## 风险与依赖

- **顺序依赖**：Phase 0 是 Phase 2/3 的地基；Phase 1 的分钟桶依赖 Redis（已有）与迁移。
- **国内厂商差异**：各家 OpenAI 兼容度不一（豆包 ark 的 usage 字段、讯飞流式协议差异），
  usage 归一化是主要返工点，先用表驱动测试锁死每家样例。
- **合规**：Guardrails 的检测规则集需要产品侧提供（敏感词/提示词注入），引擎只做框架。
- **不恢复旧配额**：产品/团队需确认预算模型（月度 vs 总额、审批流）口径，避免与钱包余额语义混淆。
- **TokenHub 借鉴**：确认许可证策略后再决定"抄思路"还是"带署名拷贝"。

## 里程碑与工作量

| 批次 | 内容 | 预估 |
|---|---|---|
| 第一批 | Phase 0 + Phase 3（资源模型 + 国内 Provider 模板 + 归一化） | 1–1.5 周 |
| 第二批 | Phase 1（RPM/TPM + 预算审批 + Guardrails） | 1 周 |
| 第三批 | Phase 2（策略路由 + 冷却 + 并发租约） | 1 周 |
| 第四批 | Phase 4（语音转写 + 视频任务，按需） | 0.5–1 周 |
| 第五批 | Phase 5（前端领域层 + 治理页面 + 模拟器 + E2E） | 2–3 周（可与第一~四批并行推进） |

## 执行路线图（Step 0–7）

> 由蓝图汇总为可执行的推进顺序；Step 7 前端与 Step 2–6 并行。

```
Step 0 准备 → Step 1 移植可 fork 资产 → Step 2 Phase0 → Step 3 Phase3
→ Step 4 Phase1 → Step 5 Phase2 → Step 6 Phase4（按需）

并行：Step 7 Phase5 前端
```

### Step 0：决策与准备（0.5 天）

1. 确认预算模型口径（月度 vs 总额、审批流）与许可证策略（已定：直接拷贝，保留 Apache-2.0
   头部与 NOTICE）。
2. 基线验证：`go build/vet/test` + 前端 `npm test/lint/build` 全绿。
3. 从 TokenHub 拷贝可 fork 资产到暂存区：`internal/billing`、`internal/guardrails`、
   `internal/perfbench`、前端 `domain/` 叶子纯函数；逐一标注依赖适配点（GORM tag、
   `core/types` 等）。

### Step 1：移植可 fork 资产（1 周）

| 子步骤 | 动作 | 验证 |
|---|---|---|
| 1a 账单同步 | OneAPI/NewAPI/阿里云同步器移植到 `internal/billing_sync/`；建表；Worker 定时同步；接对账 L3 | 真实连一家跑通一次同步 |
| 1b 内容策略 | Guardrails 引擎移植到 `internal/guardrails/`（建表 + 引擎单测；网关接线放 Step 4） | 引擎 + 检测器测试 |
| 1c 压测工具 | perfbench 落地（`tools/perfbench/`） | 对本地网关跑一次基线 |

### Step 2–6：按 Phase 0/3/1/2/4 顺序推进（见各 Phase 小节）

### Step 7：Phase 5 前端补齐（2–3 周，并行）

领域层抽离（直接搬叶子纯函数）→ 治理页面 → 路由模拟器/健康视图 → Provider 编辑器 →
Playwright E2E。

### 每批收尾门禁

- 核心路径 TDD（先 RED 后 GREEN）+ code-reviewer/security-reviewer。
- 全量验证：`go test ./... -count=1`（真实 PG）、`go vet`、`go build`、gofmt、前端
  `npm test` + `lint` + `build`；新增迁移验证 up/down 往返。
- 更新 `docs/PROJECT_STATUS.md`。

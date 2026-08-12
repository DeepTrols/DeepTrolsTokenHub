# DeepTrols AI Token 聚合平台 · 完整功能清单

> 对照 `AI聚合网关_完整文档.md`（10 篇 + 附录）逐项审计
> 审计日期: 2026-07-30
> **更新日期: 2026-08-12**（本次：AI 配额池（三层账 + 幂等）✅ 已实现；配额管理（Admin）由只读升级为创建/分配）
> 上次：2026-08-05（复核更正：TOTP / 审计日志 / 对账 L1 / 字段保护 / Worker Redis lease / 健康评分 已实现；Price Snapshot 降级为 🟡；租户隔离更正为 fail-closed）

---

## 第一篇：企业级四层架构

### 控制面 (Control Plane)
| 能力 | 架构要求 | 实现状态 |
|------|---------|---------|
| API Key 鉴权（Bearer token） | 前缀 + HMAC-SHA256 哈希 | ✅ 已实现 |
| API Key 六边界治理 | 模型/IP/累计/周/月/超限动作 | ✅ 已实现（35 测试） |
| HMAC 鉴权 | method + path + body SHA256 + ±300s + Redis nonce | ❌ 未实现 |
| JWT 用户鉴权 | HS256 + httpOnly cookie + 服务端 logout | ✅ 已实现 |
| 租户隔离 | Host header → tenant_domains 表 | ✅ 已实现（fail-closed，未知域名 403） |
| 模型目录 | 含定价 + 租户过滤 | ✅ 已实现 |
| 模型 CRUD + 多维定价 | POST/PUT/DELETE /api/admin/models | ✅ 已实现 |
| Provider 凭证管理 | 加密存储 + Sync 自动发现 + 14 家默认 URL | ✅ 已实现 |
| Channel 管理 | 绑定模型+实例、权重/健康 | ✅ 已实现 |
| 路由策略 CRUD | RoutePolicy + fallback 配置 | ✅ 已实现 |
| 租户生命周期 | 状态机（5 状态） | ✅ 已实现 |
| 配额管理 | pool / allocation / ledger 三层 + 网关 Check→429 拦截 | ✅ 已实现 |
| 用户等级折扣 | 基于 user_level 的阶梯折扣 | ❌ 未实现 |
| OEM 模型定价 | 租户级独立定价 | ❌ 未实现 |
| MFA/TOTP | 注册 + 验证 | ✅ 完整实现（login 强制 + setup + verify） |
| 登录历史 | 记录 + 查询 | ✅ 已实现 |

### 执行面 (Execution Plane)
| 能力 | 架构要求 | 实现状态 |
|------|---------|---------|
| LiteLLM 转发 | `/v1/chat/completions` | ✅ 已实现 |
| 加权最少负载路由 | weight / max_concurrency | ✅ 已实现 |
| RoutePolicy 路由 | 候选 channel + 4 fallback | ✅ 已实现 |
| Channel 实例管理 | base_url + provider_route | ✅ 已实现 |
| 流式 SSE 转发 | text/event-stream | ✅ 已实现 |
| Provider 模型同步 | 调用上游 /v1/models API 自动发现 | ✅ 已实现 |
| POST /v1/responses | OpenAI Responses API | ❌ 未实现 |
| POST /v1/messages | Anthropic Messages API | ❌ 未实现 |
| POST /v1/messages/count_tokens | Anthropic token 预估 | ❌ 未实现 |
| POST /v1/embeddings | 嵌入向量 | ❌ 未实现 |
| POST /v1/images/generations | 图片生成 | ❌ 未实现 |
| POST /v1/images/edits | 图片编辑 | ❌ 未实现 |
| POST /v1/videos/generations | Seedance 视频创建 | ❌ 未实现 |
| GET/DELETE /v1/videos/generations/:id | 视频任务查询/取消 | ❌ 未实现 |
| GET /v1/videos/generations/:id/content/:index | 视频下载 | ❌ 未实现 |
| POST /v1/providers/doubao/seedance/callback | Seedance 回调 | ❌ 未实现 |
| POST /v1/audio/transcriptions | 语音转文字 | ❌ 未实现 |
| POST /v1/audio/speech | 文字转语音 | ❌ 未实现 |
| POST /v1beta/models/{model}:generateContent | Gemini 原生图片 | ❌ 未实现 |
| Redis 实时负载追踪 | ai:channel:load 键 + Lua | ❌ 未实现（用 DB current_load） |
| 多实例并发跟踪 | INCR/DECR + 自动释放 | ❌ 未实现 |

### 资金面 (Money Plane)
| 能力 | 架构要求 | 实现状态 |
|------|---------|---------|
| 预算预留（Reserve） | 上游调用前锁预算 | ✅ 已实现 |
| Commit / Release | 成功提交，失败释放 | ✅ 已实现 |
| 乐观锁防并发 | wallet.version 字段 | ✅ 已实现 |
| 幂等扣款 | idempotency_key 唯一约束 | ✅ 已实现 |
| 9 维计费 | input/output/cache/reasoning/image/audio/tts/video | ✅ 已实现 |
| 流式计费 | SSE 最后 chunk 提取 usage | ✅ 已实现 |
| usage_log + charge_line + evidence | 三表事务写库 | ✅ 已实现 |
| Price Snapshot | 价格快照存入 usage_log | 🟡 字段落库但内容为空 map |
| Durable Outbox | 异步计费事件 + Committer 100% 测试覆盖 | ✅ 已实现 |
| 多维定价 | model_pricing 表 + conditions JSONB | 🟡 表就绪，conditions 未评估 |
| 配额消费 | 网关 Check→429 + Deduct→ledger + Restore（best-effort） | ✅ 已实现 |
| 租户级定价 | tenant_id 覆盖平台价格 | ❌ 未实现 |
| 阶梯折扣 | volume-based discount + 月度计数器 | ❌ 未实现 |
| 支付/充值 | 真实支付集成 | ❌ mock 数据 |
| 月度账单 | 汇总账单 | ❌ 未实现 |
| 余额预警 | 低于阈值通知 | ❌ 未实现 |
| FX 汇率 | 多币种支持 | ❌ 未实现 |

### 证据面 (Evidence Plane)
| 能力 | 架构要求 | 实现状态 |
|------|---------|---------|
| usage_log 记录 | 每次请求一条 | ✅ 已实现 |
| charge_line 记录 | 按维度分行 | ✅ 已实现 |
| provider_evidence 记录 | 上游原始响应 | ✅ 已实现 |
| usage 来源标记 | upstream / final_chunk / estimated | ✅ 已实现 |
| 路由证据链 | channel_id + instance_id + route_policy_id | ✅ 已实现 |
| 审计日志 | audit_logs 表 | ✅ 已实现（AuditAdminWrite 中间件） |
| 对账 L0 | usage_logs vs charge_lines 计数 | ✅ 已实现 |
| 对账 L1 | provider_evidence 匹配 | ✅ 已实现（缺口 / token 不匹配 / error 误标） |
| 对账 L2 | L0 ↔ L1 内部对账 | ❌ 未实现 |
| 对账 L3 | L1 ↔ 上游账单 | ❌ 未实现 |
| 对账 Diff 自动修复 | 修正 + 重试 | ❌ 未实现 |

---

## 第二篇：网关核心 — 16 条路由完整对照

| # | 端点 | 实现 | 计费 |
|---|------|------|------|
| 1 | `POST /v1/chat/completions` | ✅ | ✅ 完整（含配额检查） |
| 2 | `POST /v1/responses` | ❌ | — |
| 3 | `POST /v1/messages` | ❌ | — |
| 4 | `POST /v1/messages/count_tokens` | ❌ | — |
| 5 | `GET /v1/models` | ✅ | N/A |
| 6 | `POST /v1/embeddings` | ❌ | — |
| 7 | `POST /v1/images/generations` | ❌ | — |
| 8 | `POST /v1/images/edits` | ❌ | — |
| 9 | `POST /v1/videos/generations` | ❌ | — |
| 10 | `GET /v1/videos/generations/:id` | ❌ | — |
| 11 | `GET /v1/videos/generations/:id/content/:index` | ❌ | — |
| 12 | `DELETE /v1/videos/generations/:id` | ❌ | — |
| 13 | `POST /v1/providers/doubao/seedance/callback` | ❌ | — |
| 14 | `POST /v1/audio/transcriptions` | ❌ | — |
| 15 | `POST /v1/audio/speech` | ❌ | — |
| 16 | `POST /v1beta/models/{model}:generateContent` | ❌ | — |

**实现率**: 2/16（12.5%）

---

## 第三篇：执行层架构（LiteLLM + 自研 Adapter）

| 组件 | 作用 | 状态 |
|------|------|------|
| LiteLLM（代理转发） | OpenAI-compatible chat 执行 | ✅ 已接入 |
| Provider Sync | 调用上游 /v1/models API 自动发现模型 | ✅ POST /providers/{id}/sync |
| Gemini Native Adapter | 图片生成（非 chat 协议） | ❌ |
| Seedance Native Adapter | 视频创建/查询/下载/取消/回调 | ❌ |
| Audio Native Adapter | 语音转文字、文字转语音 | ❌ |

---

## 第四篇：计费引擎

| 层次 | 能力 | 状态 |
|------|------|------|
| 定价 | 模型基础定价（9 维） | ✅ |
| 定价 | 多维定价表（model_pricing） | ✅ |
| 定价 | 条件定价（conditions JSONB） | ❌ |
| 定价 | 租户级覆盖定价 | ❌ |
| 定价 | 上游成本独立记录 | ✅ |
| 折扣 | 用户等级折扣 | ❌ |
| 折扣 | OEM 独立折扣 | ❌ |
| 折扣 | 阶梯用量折扣 | ❌ |
| 配额 | quota_pools/allocations/ledger 三层 + 网关强制 | ✅ |
| 钱包 | balance + frozen + 乐观锁 | ✅ |
| 钱包 | reserve → commit/release | ✅ |
| 钱包 | 交易流水 | ✅ |
| 账本 | usage_log + charge_line 双写 | ✅ |
| 账本 | Price Snapshot | ✅ |
| 账本 | 流式计费闭环 | ✅ |
| 充值 | 支付集成 | ❌ |

---

## 第五篇：OEM 体系

| 能力域 | 状态 |
|--------|------|
| 租户创建（编码唯一、Owner 绑定、默认配置） | ✅ |
| 租户审核（pending_review → active/rejected） | ❌ |
| 租户状态（active/suspended/terminated） | ✅ |
| 租户入口（Host 优先） | ✅（fail-closed） |
| 租户上下文（tenant_id 注入全链路） | ✅ |
| 数据隔离（显式 tenant scope） | ✅ |
| Admin RBAC | ✅ |
| 模型选品（平台目录 × 租户配置） | ✅ tenant_models |
| 模型定价（租户级独立价格） | ❌ |
| PAYG 门禁（价格不完整拒绝） | ❌ |
| 客户管理（CRUD + 封禁 + 等级） | ❌ |
| AI 折扣（独立于商品折扣） | ❌ |
| 代充值（同租户钱包转账） | ❌ |
| Owner 钱包（Admin 发放额度） | ❌ |
| AI 配额池（三层账 + 幂等） | ✅ |
| API Key 代管 | ❌ 故意隐藏 |
| 可见性裁剪（OEM 经营 vs 平台成本） | ❌ |

---

## 第六篇：后端架构与高可用

| 设计 | 状态 |
|------|------|
| 模块化单体 | ✅ |
| API + Worker 双进程 | ✅ |
| Worker singleton（Redis lease） | ✅ pkg/lease（SET NX EX） |
| Durable Outbox | ✅（Committer 100% 测试覆盖） |
| Channel 健康监测 | ✅ |
| 健康评分状态机（degraded） | ✅ ±30 渐进（≥70 healthy / 30-69 degraded / <30 unhealthy） |
| Redis 实时 current_load | ❌ 用 DB |
| 共享池/独享池/混合池 | ❌ |
| RouteContext + RoutePolicy | ✅ |
| 平台层 A/B 跨 channel 重试 | ❌ |

---

## 第七篇：对账系统

| 级别 | 状态 |
|------|------|
| L0：usage_logs vs charge_lines | ✅ |
| L1：provider_evidence 匹配 | ✅ |
| L2：L0 ↔ L1 内部对账 | ❌ |
| L3：L1 ↔ 上游账单 | ❌ |
| Usage 归一化（OpenAI/Anthropic/Gemini） | ✅ |
| Anthropic cache token 解析 | ✅ |
| Gemini usageMetadata 解析 | ❌ |

---

## 第八篇：控制台

| 页面 | 状态 |
|------|------|
| 工作台 | ✅ |
| API 密钥（六边界 CRUD） | ✅ |
| 调用日志 | ✅ |
| 用量统计 | ✅ |
| 钱包账单 | ✅ |
| 模型广场 | ✅ 三级分组（商家/Plan/工厂）+ 折叠展开 |
| 在线体验（Playground） | ✅ |
| 安全设置 | ✅（TOTP login 强制 + setup + verify） |
| 开发文档 | ✅ |
| 模型管理（Admin CRUD） | ✅ Provider 下拉 + badge 同名区分 |
| Provider 凭证（Admin） | ✅ Sync 按钮 + 14 家默认 URL + 自动发现 |
| Channel 管理（Admin） | ✅ |
| 路由策略（Admin） | ✅ |
| 租户管理（Admin） | ✅ |
| 配额管理（Admin 创建/分配） | ✅ |
| 对账管理（Admin 只读） | ✅ |

---

## 第九篇：接口设计流程

| 处理步骤 | 状态 |
|---------|------|
| 入口鉴权（Bearer/HMAC/x-api-key/x-goog-api-key） | 🟡 Bearer + httpOnly cookie |
| 租户识别（Host → tenant_id） | ✅ |
| 路由决策（RouteContext + RoutePolicy） | ✅ |
| 配额检查（Check→429） | ✅ 路由后 / Reserve 前 |
| 预算预留（上游调用前 Reserve） | ✅ |
| 执行转发（LiteLLM / Native Adapter） | 🟡 仅 Chat |
| 流式 usage 提取（最后 chunk） | ✅ |
| 计费提交（同步 / 异步 Outbox） | ✅ |
| 配额扣除（Deduct→quota_ledger） | ✅ 成功后异步 best-effort |
| 幂等保护（request identity 去重） | 🟡 |
| 字段保护（拒绝覆盖 api_key/headers/base_url） | ✅（7 字段过滤） |
| Console Playground（JWT + owned key_id） | ✅ |

---

## 第十篇：OpenRouter 能力对照

| OpenRouter 能力 | DeepTrols 对照 |
|----------------|---------------|
| 统一 API 入口 | ✅ Chat（含配额拦截） |
| 统一计费 | ✅ 9 维 |
| 统一 API Key | ✅ 6 边界 |
| 模型市场 + 定价透明 | ✅ 三级分组 |
| BYOK | ❌ |
| 路由（权重 + 延迟 + 价格） | 🟡 权重 + 负载 |
| 多模态（图片/音频/视频） | ❌ |
| 组织/项目/成员/Key 治理 | ❌ |
| Workspace 隔离 | 🟡 租户雏形 |
| Agent Runtime | ❌ |

---

## 汇总

| 分类 | 总数 | ✅ 完成 | 🟡 部分 | ❌ 未开始 |
|------|------|--------|--------|----------|
| Gateway 端点 | 16 | 2 | 0 | 14 |
| 鉴权方式 | 4 | 2 | 0 | 2 |
| 计费能力 | 16 | 13 | 1 | 2 |
| 对账级别 | 4 | 2 | 0 | 2 |
| OEM 能力 | 17 | 6 | 2 | 9 |
| Console 页面 | 16 | 16 | 0 | 0 |
| Worker 能力 | 5 | 4 | 1 | 0 |
| Adapter 类型 | 4 | 1 | 0 | 3 |

### 建议实施顺序

**短期（质量加固）**:
- 前端：TanStack Query 数据层、错误边界、loading/error/empty 三态
- 后端：限流 Redis 化、reconciliation 测试、对账 L1

**中期**:
- Embeddings + Images 端点、TOTP 登录流程、前端测试补完

**长期**:
- Messages/Audio/Video 端点、Gemini Adapter、OEM 业务逻辑、BYOK、HA、支付充值

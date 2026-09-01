# DeepTrols AI Token 聚合平台 · 完整功能清单

> 对照 `AI聚合网关_完整文档.md`（10 篇 + 附录）逐项审计
> 审计日期: 2026-07-30
> **更新日期: 2026-08-12**（本次：AI 配额池（三层账 + 幂等）✅ 已实现；配额管理（Admin）由只读升级为创建/分配）
> 上次：2026-08-19（文档一致性修正：LiteLLM 移除同步 / HMAC 请求签名重新定位为可选 / Seedance 回调归类为 webhook / 健康阈值以代码为准 / Redis 自动释放语义澄清 / FX 标注按需；详见 docs/PROJECT_STATUS.md 十九节）
> 本次：2026-08-20（状态同步：Redis 实时负载追踪 / 多实例并发跟踪 / Price Snapshot / 租户审核 由 ❌、🟡 转 ✅；详见 docs/PROJECT_STATUS.md 二十三节）
> 上次：2026-08-21（B1 定价引擎落地：成本/售价分离 + 峰谷/缓存维度 + 无加价（售价=成本）+ PAYG 门禁，详见 PROJECT_STATUS.md 二十六节）
> 本次：2026-08-24（第二篇端点状态修正：/v1/embeddings、/v1/images/generations、/v1/audio/speech 已实现并注册，
> 实现率 2/16 → 5/16；详见 PROJECT_STATUS.md 二十七节）
> 本次：2026-08-25（配额管理 / 策略管理整体移除：页面、API、网关强制、route_policies 与 quota 三表随迁移 000014 删除；
> 详见 PROJECT_STATUS.md 三十节）
> 本次：2026-08-25（OEM 进阶取消：租户级定价管理入口、客户等级/AI 折扣、Owner 发额度、可见性裁剪、
> API Key 代管不再计划实施；详见 PROJECT_STATUS.md 三十二节）
> 本次：2026-08-30（端点/计费/运营状态同步：/v1/responses、/v1/messages、count_tokens、images/edits、
> videos/generations（异步任务含查询/取消）、audio/transcriptions 已实现；登录会话管理（JWT jti + 列表/撤销）、
> gzip 响应压缩、分组折扣（group_ratio + volume 阶梯）、月度账单、余额预警、订阅套餐/签到/兑换码/邀请奖励、
> 易支付（epay）M0 已接入；API Key 前缀统一 sk-；企业/团队体系已移除（/team/*、team_balance 代充值删除）。
> 详见 PROJECT_STATUS.md 九十七/九十八节）
> 修正：2026-09-01（对账 L2/L3 实际已实现：L2 = L0↔L1 交叉核对，L3 = billing_records 外部账单核对（000015）；
> 此前 2026-08-30 同步误标为 ❌，已修正；另见 PROJECT_STATUS.md 一百零五节）

---

## 第一篇：企业级四层架构

### 控制面 (Control Plane)
| 能力 | 架构要求 | 实现状态 |
|------|---------|---------|
| API Key 鉴权（Bearer token） | 前缀 + HMAC-SHA256 哈希 | ✅ 已实现 |
| API Key 六边界治理 | 模型/IP/累计/周/月/超限动作 | ✅ 已实现（35 测试） |
| HMAC 请求签名（可选） | 仅建议用于平台回调/webhook 验签；⚠️ 不适用于 OpenAI 兼容网关入口（客户端 SDK 只认 Bearer，签名会让所有兼容客户端失效） | ❌ 未实现 |
| JWT 用户鉴权 | HS256 + httpOnly cookie + 服务端 logout | ✅ 已实现 |
| 租户隔离 | Host header → tenant_domains 表 | ✅ 已实现（fail-closed，未知域名 403） |
| 模型目录 | 含定价 + 租户过滤 | ✅ 已实现 |
| 模型 CRUD + 多维定价 | POST/PUT/DELETE /api/admin/models | ✅ 已实现 |
| Provider 凭证管理 | 加密存储 + Sync 自动发现 + 14 家默认 URL | ✅ 已实现 |
| Channel 管理 | 绑定模型+实例、权重/健康 | ✅ 已实现 |
| 路由策略 CRUD | RoutePolicy + fallback 配置 | ❌ 已移除（2026-08-25） |
| 租户生命周期 | 状态机（5 状态） | ✅ 已实现 |
| 配额管理 | pool / allocation / ledger 三层 + 网关 Check→429 拦截 | ❌ 已移除（2026-08-25） |
| OEM 模型定价 | 租户级独立定价 | ✅ 数据层支持（model_pricing.tenant_id 覆盖 + 平台回退）；OEM 自助管理入口不做（2026-08-25 明确取消） |
| MFA/TOTP | 注册 + 验证 | ❌ 已移除（2026-08-11 与兑换码/邀请奖励一并删除） |
| 登录历史 | 记录 + 查询 | ✅ 已实现 |
| 登录会话管理 | 列表/撤销/撤销其他 + JWT jti | ✅ 已实现（000036_auth_sessions） |
| API Key 前缀 | 统一 `sk-`（2026-08-26 去掉 `dt-`） | ✅ 已实现 |

### 执行面 (Execution Plane)
| 能力 | 架构要求 | 实现状态 |
|------|---------|---------|
| OpenAI 兼容直连转发 | `/v1/chat/completions` | ✅ 已实现（渠道实例直连上游；内置 LiteLLM 已于 2026-08-19 移除） |
| 加权最少负载路由 | weight / max_concurrency | ✅ 已实现 |
| RoutePolicy 路由 | 候选 channel + 4 fallback | ❌ 已移除（路由回退为全部健康渠道按权重/负载排序） |
| Channel 实例管理 | base_url + provider_route | ✅ 已实现 |
| 流式 SSE 转发 | text/event-stream | ✅ 已实现 |
| Provider 模型同步 | 调用上游 /v1/models API 自动发现 | ✅ 已实现 |
| POST /v1/responses | OpenAI Responses API | ✅ 已实现（直连上游 /v1/responses；chat-only 渠道自动转换，见 responses_via_chat） |
| POST /v1/messages | Anthropic Messages API | ✅ 已实现（OpenAI SSE ⇄ Anthropic Messages SSE） |
| POST /v1/messages/count_tokens | Anthropic token 预估 | ✅ 已实现（免费，不路由不计费） |
| POST /v1/embeddings | 嵌入向量 | ✅ 已实现（转发 + 计费闭环） |
| POST /v1/images/generations | 图片生成 | ✅ 已实现（转发 + 计费闭环） |
| POST /v1/images/edits | 图片编辑 | ✅ 已实现（multipart 管线） |
| POST /v1/videos/generations | Seedance 视频创建 | ✅ 已实现（异步任务：创建后查询/取消） |
| GET/DELETE /v1/videos/generations/:id | 视频任务查询/取消 | ✅ 已实现（content/:index 下载未提供） |
| GET /v1/videos/generations/:id/content/:index | 视频下载 | ❌ 未实现 |
| POST /v1/providers/doubao/seedance/callback | Seedance 回调（⚠️ 上游→平台的 webhook，非客户端 API：需独立验签，不应走 /v1 Bearer 鉴权） | ❌ 未实现 |
| POST /v1/audio/transcriptions | 语音转文字 | ✅ 已实现（multipart 管线） |
| POST /v1/audio/speech | 文字转语音 | ✅ 已实现（raw 转发 + TTS 字符计费） |
| POST /v1beta/models/{model}:generateContent | Gemini 原生端点 | ❌ 未提供（内部 GeminiAdapter 已支持 chat → generateContent 转换） |
| Redis 实时负载追踪 | ai:channel:load 键 + Lua | ✅ 已实现（2026-08-19 LoadTracker：请求开始 INCR / 结束 DECR；Redis 故障回退 DB current_load + 每分钟限流告警，不静默降级） |
| 多实例并发跟踪 | INCR/DECR + 显式 DECR（defer 兜底）+ 心跳刷新 TTL（⚠️ 不能只靠 TTL 过期，否则活跃计数器会被清零、负载信号失真） | ✅ 已实现（显式 DECR + defer 兜底，双释放不为负；心跳每 TTL/2 刷新，进程崩溃计数随 TTL 自动消失） |

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
| Price Snapshot | 价格快照存入 usage_log | ✅ 已实现（pricer 写入 price_version / source / currency / captured_at / rows，非空快照） |
| 成本/售价分离 + 峰谷定价 | cost/sell × peak/off_peak 定价行；售价 = 显式售价行 或 成本原价（无加价）；缓存命中按 cache_read 维度计费 | ✅ 已实现（migration 000011 + pricer 双通道） |
| Durable Outbox | 异步计费事件 + Committer | ❌ 已移除（计费已同步化；Committer 已删，outbox_events 表随迁移 000013 正式删除） |
| 多维定价 | model_pricing 表 + conditions JSONB | ✅ 已实现（000034 pricing_tiers：按 max_total_tokens 匹配阶梯价） |
| 配额消费 | 网关 Check→429 + Deduct→ledger + Restore（best-effort） | ❌ 已移除（2026-08-25） |
| 租户级定价 | tenant_id 覆盖平台价格 | ✅ 数据层支持（tenant_id 优先、NULL 回退平台价）；管理入口不做（2026-08-25 明确取消） |
| 阶梯折扣 | volume-based discount + 月度计数器 | ✅ 已实现（分组倍率 group_ratio + 月度累计 volume 阶梯；price_snapshot 记录 price_ratio） |
| 支付/充值 | 真实支付集成 | 🟡 易支付（epay）M0 已接入（下单/回调验签 + 幂等）；官方渠道适配器待接 |
| 月度账单 | 汇总账单 | ✅ 已实现（GET /api/console/billing/statement，GMT+8 自然月） |
| 余额预警 | 低于阈值通知 | ✅ 已实现（wallet/alert 阈值） |
| 订阅套餐 | 套餐 CRUD + 购买 + 订单 + 自动续费 + 过期回收 | ✅ 已实现（000028-000032） |
| 签到 / 兑换码 / 邀请奖励 | 每日签到、兑换码、邀请返利 | ✅ 已实现（000025/000026/000033） |
| 响应缓存失效 | 渠道凭据更新时清空 Redis 响应缓存 | ✅ 已实现（cache.Service.Purge，2026-08-26） |
| FX 汇率 | 多币种支持（⚠️ 国内 CNY 场景暂非必需，标注为按需启用，非 MVP 必做） | ❌ 未实现 |

### 证据面 (Evidence Plane)
| 能力 | 架构要求 | 实现状态 |
|------|---------|---------|
| usage_log 记录 | 每次请求一条 | ✅ 已实现 |
| charge_line 记录 | 按维度分行 | ✅ 已实现 |
| provider_evidence 记录 | 上游原始响应 | ✅ 已实现 |
| usage 来源标记 | upstream / final_chunk / estimated | ✅ 已实现 |
| 路由证据链 | channel_id + instance_id | ✅ 已实现 |
| 审计日志 | audit_logs 表 | ✅ 已实现（AuditAdminWrite 中间件） |
| 对账 L0 | usage_logs vs charge_lines 计数 | ✅ 已实现 |
| 对账 L1 | provider_evidence 匹配 | ✅ 已实现（缺口 / token 不匹配 / error 误标） |
| 对账 L2 | L0 ↔ L1 内部对账 | ✅ 已实现（象限计数 + balanced 判定） |
| 对账 L3 | 外部账单（billing_records）↔ 内部 usage | ✅ 已实现（billing_without_usage / usage_without_billing / amount_mismatch） |
| 对账 Diff 自动修复 | 修正 + 重试 | ❌ 未实现 |

---

## 第二篇：网关核心 — 16 条路由完整对照

| # | 端点 | 实现 | 计费 |
|---|------|------|------|
| 1 | `POST /v1/chat/completions` | ✅ | ✅ 完整 |
| 2 | `POST /v1/responses` | ✅ | ✅ 完整（直连 /v1/responses；chat-only 渠道自动转换） |
| 3 | `POST /v1/messages` | ✅ | ✅ 完整（SSE 格式转换 + 计费闭环） |
| 4 | `POST /v1/messages/count_tokens` | ✅ | N/A 免费（不路由不计费） |
| 5 | `GET /v1/models` | ✅ | N/A |
| 6 | `POST /v1/embeddings` | ✅ | ✅ 完整（估算 token 计费） |
| 7 | `POST /v1/images/generations` | ✅ | ✅ 完整（按图计费） |
| 8 | `POST /v1/images/edits` | ✅ | ✅ 完整（multipart 管线） |
| 9 | `POST /v1/videos/generations` | ✅ | ✅ 完整（异步任务 + 预留生命周期） |
| 10 | `GET /v1/videos/generations/:id` | ✅ | N/A（任务状态查询） |
| 11 | `GET /v1/videos/generations/:id/content/:index` | ❌ | — |
| 12 | `DELETE /v1/videos/generations/:id` | ✅ | N/A（任务取消） |
| 13 | `POST /v1/providers/doubao/seedance/callback` | ❌ | — |
| 14 | `POST /v1/audio/transcriptions` | ✅ | ✅ 完整（multipart 管线） |
| 15 | `POST /v1/audio/speech` | ✅ | ✅ 完整（TTS 字符计费） |
| 16 | `POST /v1beta/models/{model}:generateContent` | ❌ | — |

**实现率**: 13/16（81.3%）

---

## 第三篇：执行层架构（OpenAI 兼容直连 + 自研 Adapter）

| 组件 | 作用 | 状态 |
|------|------|------|
| OpenAI 兼容直连 | OpenAI-compatible chat 执行 | ✅ 已实现（内置 LiteLLM 已于 2026-08-19 移除） |
| Channel Adapter（Anthropic/Azure/Ollama/Custom/Gemini） | `upstream_format` 选择转换器（OpenAI chat ⇄ 各协议） | ✅ 已实现 |
| Provider Sync | 调用上游 /v1/models API 自动发现模型 | ✅ POST /providers/{id}/sync |
| Gemini Native Adapter | /v1beta 原生端点（非 chat 协议） | ❌ 未提供；内部 GeminiAdapter 已支持 chat → generateContent 转换 |
| Seedance Native Adapter | 视频创建/查询/下载/取消/回调 | 🟡 创建/查询/取消已实现（OpenAI 兼容直连异步任务）；content/:index 下载与回调未提供 |
| Audio Native Adapter | 语音转文字、文字转语音 | ✅ multipart 管线（无独立 native adapter） |

---

## 第四篇：计费引擎

| 层次 | 能力 | 状态 |
|------|------|------|
| 定价 | 模型基础定价（9 维） | ✅ |
| 定价 | 多维定价表（model_pricing） | ✅ |
| 定价 | 条件定价（conditions JSONB） | ✅ |
| 定价 | 租户级覆盖定价 | 🟡 数据层已支持（model_pricing.tenant_id），管理入口未提供 |
| 定价 | 上游成本独立记录 | ✅ |
| 折扣 | 阶梯用量折扣（分组倍率 + volume 阶梯） | ✅ |
| 配额 | quota_pools/allocations/ledger 三层 + 网关强制 | ❌ 已移除（2026-08-25） |
| 钱包 | balance + frozen + 乐观锁 | ✅ |
| 钱包 | reserve → commit/release | ✅ |
| 钱包 | 交易流水 | ✅ |
| 账本 | usage_log + charge_line 双写 | ✅ |
| 账本 | Price Snapshot | ✅ |
| 账本 | 流式计费闭环 | ✅ |
| 充值 | 支付集成（易支付 M0；官方渠道待接） | 🟡 |
| 账本 | 月度账单（GMT+8 自然月聚合） | ✅ |
| 钱包 | 余额预警阈值 | ✅ |
| 钱包 | 订阅套餐（购买/订单/自动续费/过期回收） | ✅ |
| 钱包 | 签到 / 兑换码 / 邀请奖励入账 | ✅ |

---

## 第五篇：OEM 体系

| 能力域 | 状态 |
|--------|------|
| 租户创建（编码唯一、Owner 绑定、默认配置） | ✅ |
| 租户审核（pending_review → active/rejected） | ✅ |
| 租户状态（active/suspended/terminated） | ✅ |
| 租户入口（Host 优先） | ✅（fail-closed） |
| 租户上下文（tenant_id 注入全链路） | ✅ |
| 数据隔离（显式 tenant scope） | ✅ |
| Admin RBAC | ✅ |
| 模型选品（平台目录 × 租户配置） | ✅ tenant_models |
| 模型定价（租户级独立价格） | ✅ 数据层已支持（tenant_id 覆盖 + 平台回退）；OEM 自助管理入口不做 |
| PAYG 门禁（价格不完整拒绝） | ✅ 网关估算阶段 422 pricing_incomplete；结算阶段按预留额计费并留 evidence（pricing_incomplete） |
| 客户管理（CRUD + 封禁 + 角色） | ❌ 已移除（2026-08-25 企业/团队体系收敛，/team/* 子账号删除；等级不做） |
| 代充值（同租户钱包转账） | ❌ 已移除（2026-08-25 随团队体系删除） |

---

## 第六篇：后端架构与高可用

| 设计 | 状态 |
|------|------|
| 模块化单体 | ✅ |
| API + Worker 双进程 | ✅ |
| Worker singleton（Redis lease） | ✅ pkg/lease（SET NX EX） |
| Durable Outbox | ❌ 已移除（计费同步化；表随迁移 000013 删除） |
| Channel 健康监测 | ✅ |
| 健康评分状态机（degraded） | ✅ ±30 渐进（≥70 healthy / 30-69 degraded / <30 unhealthy） |
| Redis 实时 current_load | ✅ LoadTracker（Redis 优先，故障回退 DB + 限流告警） |
| 共享池/独享池/混合池 | ❌ |
| RouteContext（候选排序 + 实例选择） | ✅（RoutePolicy 已移除） |
| 平台层 A/B 跨 channel 重试 | ✅ 已实现（候选 failover：RouteCandidates 顺序重试，失败自动切换下一渠道） |

---

## 第七篇：对账系统

| 级别 | 状态 |
|------|------|
| L0：usage_logs vs charge_lines | ✅ |
| L1：provider_evidence 匹配 | ✅ |
| L2：L0 ↔ L1 内部对账 | ✅ |
| L3：外部账单（billing_records）↔ 内部 usage | ✅ |
| Usage 归一化（OpenAI/Anthropic/Gemini） | ✅ |
| Anthropic cache token 解析 | ✅ |
| Gemini usageMetadata 解析 | ❌ |

---

## 第八篇：控制台

| 页面 | 状态 |
|------|------|
| 工作台（Dashboard） | ✅ |
| API 密钥（六边界 CRUD） | ✅ |
| 调用日志（用量信息 + 按模型查看） | ✅ |
| 月度账单（Bills） | ✅ |
| 充值（Recharge） | ✅ 易支付 M0 |
| 订阅（Subscriptions） | ✅ 套餐/购买/自动续费 |
| 模型广场（ModelMarket） | ✅ 平铺列表 + 定价 |
| 在线体验（Playground） | ✅ |
| 用户中心（UserCenter） | ✅ 账户资料 + 安全设置 + 登录历史 + 会话管理 |
| 排行榜（Rankings）/ 价格（Pricing）/ 开发文档（Docs）/ 关于 / 法律 | ✅ |
| 模型管理（Admin CRUD） | ✅ Provider 下拉 + badge 同名区分 |
| Provider 凭证（Admin） | ✅ Sync 按钮 + 14 家默认 URL + 自动发现 |
| Channel 管理（Admin） | ✅ 实例增删/测试/批量测试/模型绑定 |
| 租户管理（Admin） | ✅ |
| 用户管理（Admin） | ✅ |
| 对账管理（Admin 只读） | ✅ |
| 审计日志（Admin） | ✅ |
| 网关健康（Admin） | ✅ 渠道/实例实时健康总览 |
| 兑换码 / 订阅套餐 / 订阅管理（Admin） | ✅ |
| 系统设置（Admin） | ✅ 站点/计费/模型/请求限制/安全/运营/系统信息分区 |

---

## 第九篇：接口设计流程

| 处理步骤 | 状态 |
|---------|------|
| 入口鉴权（Bearer/HMAC/x-api-key/x-goog-api-key） | 🟡 Bearer + httpOnly cookie |
| 租户识别（Host → tenant_id） | ✅ |
| 路由决策（RouteContext：候选排序 + 实例选择） | ✅ |
| 配额检查（Check→429） | ❌ 已移除（2026-08-25） |
| 预算预留（上游调用前 Reserve） | ✅ |
| 执行转发（OpenAI 兼容直连 / Channel Adapter） | ✅ Chat + Responses/Messages + 多模态 |
| 流式 usage 提取（最后 chunk） | ✅ |
| 计费提交 | ✅ 同步（Reserve→Settle→Release 单请求内完成；Outbox 已移除） |
| 幂等保护（request identity 去重） | ✅ pkg/idempotency + 支付幂等 |
| 字段保护（拒绝覆盖 api_key/headers/base_url） | ✅（7 字段过滤） |
| Console Playground（JWT + owned key_id） | ✅ |

---

## 第十篇：OpenRouter 能力对照

| OpenRouter 能力 | DeepTrols 对照 |
|----------------|---------------|
| 统一 API 入口 | ✅ Chat / Responses / Messages |
| 统一计费 | ✅ 9 维 |
| 统一 API Key | ✅ 6 边界 |
| 模型市场 + 定价透明 | ✅ 平铺列表 |
| BYOK | ❌ |
| 路由（权重 + 延迟 + 价格） | 🟡 权重 + 负载 |
| 多模态（图片/音频/视频） | ✅ 图片生成/编辑、音频 TTS/转写、视频异步生成 |
| 组织/项目/成员/Key 治理 | ❌ |
| Workspace 隔离 | 🟡 租户雏形 |
| Agent Runtime | ❌ |

---

## 汇总

| 分类 | 总数 | ✅ 完成 | 🟡 部分 | ❌ 未开始 |
|------|------|--------|--------|----------|
| Gateway 端点 | 16 | 13 | 0 | 3 |
| 鉴权方式 | 4 | 2 | 1 | 1 |
| 计费能力 | 18 | 17 | 1 | 0 |
| 对账级别 | 4 | 4 | 0 | 0 |
| OEM 能力 | 12 | 10 | 0 | 2（已移除） |
| Console 页面 | 34 | 34 | 0 | 0 |
| Worker 能力 | 5 | 5 | 0 | 0 |
| Adapter 类型 | 6 | 4 | 2 | 0 |

### 建议实施顺序

**短期（质量加固）**:
- 对账差异自动修复（L2/L3 已实现）；监控指标与告警（Prometheus + 告警规则）
- 干净环境部署演练 + 备份恢复演练（见 PRODUCTION_READINESS B7/B8）

**中期**:
- 官方支付渠道（支付宝/微信）适配器，替代易支付 M0；视频下载 content/:index 与 Seedance 回调验签
- 多币种/FX、BYOK

**长期**:
- /v1beta Gemini 原生端点、对账 L3 上游账单核对、公开市场数据报告

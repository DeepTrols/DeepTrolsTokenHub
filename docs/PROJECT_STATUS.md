# DeepTrols AI Token 聚合平台 · 项目进度报告

> 报告日期: 2026-08-04

---

## 一、项目概况

企业级 AI Token 聚合平台。不是反向代理，是围绕模型调用构建的计费、风控、对账与运营系统。Go 1.22 + PostgreSQL 16 + Redis 7 + React 18 + TypeScript。

---

## 二、已完成功能

### 2.1 计费引擎

| 功能 | 说明 |
|------|------|
| Reserve-Commit-Release | 上游调用前锁预算，成功提交，失败释放 |
| 乐观锁 | wallet.version 字段防并发 |
| 幂等保护 | idempotency_key 唯一约束，防重复扣费 |
| 9 维定价 | input/output/cache_read/cache_write/reasoning/image/audio/tts/video |
| decimal 精度 | 所有金额用 decimal，无浮点数 |
| 三表事务写入 | usage_log + charge_line + provider_evidence 单事务落库 |
| 流式计费闭环 | SSE 最后 chunk 提取 usage，detached context 提交 |
| 计费同步化 | Reserve → Settle（按真实用量多退少补）→ Release 单请求内完成 |
| 配额强制 | 网关 Check→429 拦截，Deduct→ledger 扣除，Restore 失败恢复 |
| 响应缓存 | SHA256(request)→Redis，命中零计费，X-Cache: HIT |

### 2.2 5 不变量

| # | 不变量 | 实现 |
|---|--------|------|
| 1 | request_id 不是全局唯一账务身份 | 复合身份 tenant+user+key+type+request_id |
| 2 | 预算预留在上游调用前 | Reserve → Execute → Commit/Release |
| 3 | 路由结果进入证据链 | channel_id + instance_id + route_policy_id |
| 4 | usage 来源显式标记 | upstream / estimated / cached 三种标记 |
| 5 | 流式错误不伪装成功 | ⚠️ 有缺陷，[DONE] 无条件发送 |

### 2.3 网关

| 端点 | 状态 |
|------|------|
| POST /v1/chat/completions（非流式） | ✅ 完整计费链路 |
| POST /v1/chat/completions（流式 SSE） | ✅ 完整计费链路 |
| GET /v1/models | ✅ 按 API Key allowlist 过滤 |
| 请求字段保护 | ✅ 过滤 api_key/headers/base_url 等 7 个字段 |

### 2.4 鉴权与安全

| 能力 | 说明 |
|------|------|
| API Key Bearer Token | HMAC-SHA256 哈希查表，6 边界治理（模型/IP/累计/周/月/超限动作） |
| JWT 用户鉴权 | HS256，httpOnly cookie + SameSite，服务端 logout |
| TOTP 2FA | 完整实现（setup + verify + login 强制），RFC 6238 |
| 租户识别 | Host header → tenant_domains，fail-closed（未知域名 403） |
| 安全头 | CSP/HSTS/X-Frame-Options/X-Content-Type-Options/Referrer-Policy |
| 限流 | Redis 优先 + 内存降级，网关按 Key 限流，登录按 IP 限流 |

### 2.5 管理后台 API

| 模块 | 接口 |
|------|------|
| 模型管理 | CRUD + 多维定价 |
| Provider 凭证 | CRUD + 14 家默认 URL + Sync 自动发现模型 |
| 渠道管理 | CRUD + 实例管理（添加/删除）+ 健康/权重 |
| 路由策略 | CRUD + 4 种 fallback 策略 |
| 租户管理 | CRUD + 域名管理 + 5 状态状态机 |
| 配额管理 | 池创建 + 用户分配 + 账簿查询 |
| 用户管理 | 列表 + 创建 + 角色/状态编辑 + 删除 |
| 成本分析 | 按模型成本汇总 + 加价率设置 |
| 对账管理 | 查看对账运行记录与差异 |
| 审计日志 | 全量 Admin 操作记录 |

### 2.6 用户控制台 API (23/23)

登录、注册、TOTP 设置/验证、登出、个人信息、修改密码、API Key CRUD（6 边界）、密钥明文查看、用量历史（含费用明细）、钱包余额、交易记录、在线充值、兑换码、邀请码、模型列表、登录历史

### 2.7 Worker 后台任务

| Worker | 说明 |
|--------|------|
| Health Checker | 每 60s 探测所有渠道实例 /health 端点 |
| Reconciler | 每 1h 运行 L0（漏记账）+ L1（证据不匹配）对账 |

### 2.8 前端

| 项目 | 说明 |
|------|------|
| UI 框架 | shadcn/ui（Radix 原语），21 页面全部迁移 |
| 页面结构 | SectionPageLayout 统一组件 |
| 状态组件 | LoadingState / ErrorState / EmptyState |
| 用户端页面 | 数据看板、API 密钥、模型广场、在线体验、调用记录、钱包管理、用量统计、安全设置、开发文档 |
| 管理端页面 | 模型管理、渠道管理、租户管理、路由策略、配额管理、对账管理、审计日志、用户管理、成本分析 |

### 2.9 部署与运维

| 项目 | 说明 |
|------|------|
| Docker | docker compose up -d 一键启动 6 容器（api + worker + web + postgres + redis + litellm） |
| 热重载 | Air（Go）+ Vite HMR（前端），代码保存即生效 |

---

## 三、未完成功能

### 3.1 架构文档明确要求（需要做）

| # | 功能 | 当前状态 | 预计 |
|---|------|---------|------|
| 1 | 流式错误不伪装成功 | [DONE] 无条件发送，usage log 始终 completed | 1天 |
| 2 | HMAC 认证 | 只有 Bearer Token，文档要求 method+path+body SHA256 + 时间窗口 + nonce | 2-3天 |
| 3 | 折扣引擎 | DiscountAmount/DiscountApplied 字段存在，计算逻辑为空 | 2周 |
| 4 | Worker 分布式选主 | 无 Redis lease，多实例重复执行 health check + 对账 | 1天 |

### 3.2 有代码但不符合文档要求

| # | 功能 | 当前 | 文档要求 | 预计 |
|---|------|------|---------|------|
| 5 | 健康检查评分 | 只有 healthy/unhealthy 二级 | 渐进：<30 degraded，>70 recovering | 1天 |
| 6 | 路由负载计数 | 读 DB current_load | 必须用 Redis INCR/DECR + Lua | 1天 |
| 7 | final_chunk 标记 | 常量定义但从未使用 | 流式应标记 final_chunk | 0.5天 |
| 8 | 租户 DB 故障 | 穿透到平台 | 未知域名不能落到平台 | 0.5天 |
| 9 | 无钱包用户 | 跳过 reserve 直接调用 | 必须拦截 | 0.5天 |
| 10 | 价格快照 | 始终为空 map | 记录定价版本和数据来源 | 0.5天 |

### 3.3 OEM 体系缺失

| # | 功能 |
|---|------|
| 11 | 租户客户管理（封禁/调级/代充值） |
| 12 | 租户创建时初始化 brand_config / runtime_config |
| 13 | OEM 自助模型定价管理 |
| 14 | 代充值（同租户钱包转账） |

### 3.4 MVP 范围外

| 类别 | 内容 |
|------|------|
| 网关扩展端点 | embeddings / images / audio / video / Gemini 等 14 个端点 |
| 对账升级 | L2（L0↔L1 内部对账）+ L3（L1↔上游账单） |
| 支付集成 | Stripe / 支付宝 / 微信 |
| 多币种 | FX 汇率 + 多币种钱包 |
| 竞品功能 | CoAI 聊天 UI、New API 格式转换管道、Advanced Custom Channel |

---

## 四、与其他项目对比

| 维度 | DeepTrols | CoAI | New API |
|------|-----------|------|---------|
| 资金面（计费/对账/5 不变量） | ⭐⭐⭐ | ⭐ | ⭐⭐ |
| 网关面（Provider/格式转换） | ⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| 用户体验（聊天 UI/订阅/支付） | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| 工程质量（测试/幂等/精度） | ⭐⭐⭐ | ⭐ | ⭐⭐ |
| 多租户 | ⭐⭐⭐ | ❌ | ❌ |
| Provider 覆盖 | LiteLLM 间接 | 17+ 内置 | 50+ 内置 |

---

## 五、建议实施计划

| 阶段 | 时间 | 内容 |
|------|------|------|
| 本周 | 2天 | 流式错误修复 + 租户DB故障 + 无钱包拦截 |
| 两周内 | 6天 | HMAC 认证 + Worker选主 + 健康评分 + 路由Redis |
| 一月内 | 3-4周 | 折扣引擎 + OEM客户管理 |
| 后续 | 按需 | 支付网关 → 网关扩展端点 → 多币种 |

---

## 六、2026-08-05 修复记录

> 追加记录（不修改上文旧记录）。当日定位并修复了**三层相互掩盖的网关故障**，Playground 从"Invalid API key"到模型调用完全打通。

### 修复内容

| # | 现象 | 根因 | 修复 |
|---|------|------|------|
| 1 | Playground 报 "Invalid API key" | 前端选择 key 时先把 **key UUID** 当 Bearer 发送，真实明文（secret 接口）尚未加载 | `web/src/pages/Playground.tsx`：移除 UUID fallback，模型请求等真实明文后再发 |
| 2 | 报 "Unknown tenant domain" | `FindByDomain` 把 `pgx.ErrNoRows` 当 DB 故障 → 平台 host（`api`/`localhost`）被 fail-closed 拒绝 | `internal/repository/tenant/postgres.go`：无租户绑定返回 `(nil, nil)`，与真 DB 故障区分 |
| 3 | 报 "No provider channel" | 健康检查要求 `{base_url}/health` 返回 2xx，但 OpenAI 兼容上游返回 401/404 → 38 个渠道全标不健康 | `internal/worker/health_checker/checker.go`：2xx/3xx/4xx 视为可达，仅 5xx/传输错误为不健康 |

### 附带修正

- **worker 容器入口**：`docker-compose.yml`/`docker-compose.dev.yml` 中 worker 服务 `command: ["./worker"]` → `entrypoint: ["./worker"]`（Dockerfile ENTRYPOINT 是 `./server`，原写法实际执行 `./server ./worker`，worker 逻辑从未运行）。此修复使 **health_checker 首次真正运行**，从而暴露了第 3 项探针 bug。
- **api 容器 Redis**：`docker-compose.dev.yml` 的 api 服务显式 `REDIS_URL: redis://redis:6379/0`（容器内 `.env` 的 localhost 指向自身）。

### 验证

- `go build ./...` 通过；health_checker / tenant / middleware 测试全绿；Playground 15 个组件测试通过。
- 38 个渠道修复后保持 healthy（health_checker 跑完一轮仍健康）。
- 浏览器端到端：模型列表加载 → deepseek-v4-flash → 返回真实响应。

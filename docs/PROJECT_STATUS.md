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
| 配额管理 | 池 CRUD（创建/编辑/删除）+ 用户分配 + 账簿查询 |
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


## 七、2026-08-10 修复记录

> 前端「渠道管理」页面 API 端点修正 + 开发环境配置补全。用户在前端界面添加 URL 和 API Key 后，系统自动发现该 Provider 的模型列表。

### 修复内容

| # | 现象 | 根因 | 修复 |
|---|------|------|------|
| 4 | 前端渠道管理「添加渠道」提交后无模型自动发现 | `Channels.tsx` 的 CRUD mutation 全部指向 `/channels` 端点，该端点要求前端先选择已存在的 `model_id`，没有自动发现链路 | `web/src/pages/Channels.tsx`：3 个 mutation 端点从 `/channels` 改为 `/providers`（`POST /api/admin/providers` 会先调上游 `/v1/models` 自动发现模型，再创建 models + channels + channel_instances） |
| 5 | 本地启动 Go API 进程拒绝启动 | production 模式下检测到弱 JWT secret 硬退出 | `.env` 添加 `ENABLE_FAKE_PAYMENT=true`，开发模式允许弱密钥（生产必须为 false） |
| 6 | Vite 前端 dev server 请求被 CORS 拦截 | `.env` 中 `CORS_ORIGIN` 只含 `localhost:3000`（Docker web 容器端口），不含 Vite 默认端口 | `.env` 的 `CORS_ORIGIN` 追加 `http://localhost:5173` |

### 技术说明

**Provider vs Channel 端点区别：**

| 端点 | 用途 | 自动发现模型 |
|------|------|-------------|
| `POST /api/admin/providers` | 创建 Provider 凭证（name + provider type + api_key + base_url） | ✅ 调用上游 `/v1/models` 自动发现，创建 models + channels + instances |
| `POST /api/admin/channels` | 为已存在的模型创建渠道 | ❌ 需要 `model_id`，模型必须已存在数据库中 |

前端「渠道管理」页面的交互意图是：用户选择服务商类型 → 填入 API Key 和 Base URL → 提交 → 系统自动拉取该服务商的模型列表。这匹配 `/providers` 端点的语义。`/channels` 端点是底层渠道创建 API，不面向此交互场景。

### 已知限制

- **LiteLLM 无上游 API Key**：`litellm-config.yaml` 中引用了 `${OPENAI_API_KEY}` 和 `${ANTHROPIC_API_KEY}`，但未在 docker-compose 或 `.env` 中设置。LiteLLM 容器可启动但模型列表为空。直连 Provider 的模型发现不受影响（Go API 直接调用上游 `/v1/models`，不经过 LiteLLM）。


## 八、2026-08-10 四用户类型 Phase 2 完成记录

> ⚠️ **本节为历史快照，部分内容已被 2026-08-11/12 重构取代**：企业设置（EnterpriseSettings）、团队邀请、转让所有权已在 `b85d4fa` / `04dbe7b` 中删除；企业/团队能力改为「平台租户 + 子账号 + 配额分配」模型，见第九节。

> 四类用户（系统管理员 / 企业管理员 / 企业成员 / 个人用户）前后端完整落地。个人用户与企业成员共享控制台基础能力；企业管理员额外获得企业设置 + 团队管理；系统管理员拥有全局管理后台。

### 后端新增/扩展

| # | 变更 | 文件 |
|---|------|------|
| 1 | Admin 租户成员管理 API（列表/添加/移除/改角色） | `internal/handler/console/admin_memberships.go` |
| 2 | User Ledger 响应扩展（user_type / tenant_id / tenant_name） | `internal/handler/console/ledger.go` |
| 3 | 企业设置 API（GET/PUT `/enterprise` + PUT `/enterprise/brand`） | `internal/handler/console/enterprise.go` |
| 4 | Team 端点扩展（成员状态、待处理邀请、取消邀请、转让所有权） | `internal/handler/console/team.go` |
| 5 | 新路由注册（`/team/invitations`、`/team/transfer-ownership` 等） | `cmd/api/main.go` |

### 前端

| 页面 | 说明 |
|------|------|
| `Users.tsx` | 新增 user_type / 所属企业列与筛选 |
| `Tenants.tsx` | 新增成员数量 + 成员管理入口 |
| `TenantMembers.tsx` | 新增管理员视角企业成员管理页（`/admin/tenants/:id/members`） |
| `EnterpriseSettings.tsx` | 新增企业设置页（`/enterprise`，owner/admin 分级权限） |
| `TeamManagement.tsx` | 扩展：待处理邀请、取消邀请、转让所有权、成员停用/启用；修复 `/team` 响应结构读取 bug（`{members}` 而非 `{data}`） |
| `AdminLayout.tsx` | 侧边栏新增「租户管理」「策略管理」 |
| `ConsoleLayout.tsx` | 企业管理员可见「企业设置」「团队管理」 |

### 验证（全绿）

- `go build ./...` ✅ · `go vet ./...` ✅
- `go test -p 1 -count=1 ./...` ✅ 全部通过
- `npx tsc --noEmit` ✅ · `npm run build`（tsc -b + vite build）✅

### 测试修复

| 文件 | 修复 |
|------|------|
| `internal/repository/user/postgres.go` | `Create` 归一化空 `user_type` → `personal`，避免显式 `''` 违反 `users_user_type_check` |
| `internal/repository/membership/repository_test.go` | `seedTenant` 补充唯一 `code`（`tenants.code NOT NULL UNIQUE`） |
| `internal/repository/invitation/repository_test.go` | `seedTenantForInvitation` 补充唯一 `code` |

### 已知限制

- **Go 测试套件需 `-p 1` 串行运行**：多个 repository 测试包共享同一 Postgres 库并并发 TRUNCATE 相同表（users 等），默认并行下会触发 `40P01 deadlock detected`。`go test -p 1 ./...` 可避免。彻底解决需每包独立 schema，属后续优化。

### 安全评审修正（2026-08-10）

Phase 2 团队/企业代码经 **security-reviewer** 全面审计：授权模型结论为「权限矩阵正确、租户隔离健壮、无 CRITICAL/HIGH」。按评审修正以下 MEDIUM/LOW 项：

| 严重级 | 发现 | 修复 |
|--------|------|------|
| MEDIUM | `user/postgres.go` Create 内 `user.UserType = userType` 就地修改调用方结构，违反不可变性原则 | 改为仅本地变量归一化，插入用局部值，不再写回调用方指针 |
| MEDIUM | `HandleChangeMemberRole` 未校验目标成员状态，可修改停用/已离开成员角色 | 新增 `m.Status != active` 拦截（400） |
| MEDIUM | 团队管理端点无限流，可刷邀请/状态切换/转让 | 新增 `middleware.TeamRateLimit`（按 ConsoleAuth 用户 ID 限流，IP 兜底），应用于 `/team` 路由子组（30 次/分钟），含单元测试 |
| LOW | 邀请邮箱仅用 `strings.Contains("@")` 校验 | 改用 `mail.ParseAddress` 严格校验，与注册端一致 |
| LOW | 前端直接渲染后端错误消息，可能泄露角色层级 | 保留：错误均为受控字符串，且对管理员有诊断价值；未做通用化映射 |

**未采纳项（记录）：**
- `M-4`：建议把租户鉴权从「每 handler 显式调用」上移到路由级中间件。当前各 handler 均已独立正确调用 `isTenantAdmin/isTenantOwner`，且既有审计已确认无越权；统一中间件属防御性重构，列为后续优化。
- `L-3`：`HandleRemoveMember` 未校验目标状态——删除已离开成员的陈旧记录本身幂等无害，保留现状。


## 九、2026-08-11/12 企业/团队体系重构 + 配额池修复记录

> 企业/团队能力不再走「独立企业设置 + 邀请 + 转让所有权」路径，改为 **平台租户 + 子账号 + 配额分配** 的简化模型。本节取代第八节中关于企业设置/邀请/转让的描述（见第八节 ⚠️ 标注）。

### 9.1 后端变更

| # | 变更 | 提交 |
|---|------|------|
| 1 | 删除 enterprise 端点、invitation、transfer-ownership（及对应 handler/repository） | `b85d4fa` |
| 2 | 删除 EnterpriseSettings，重做 TeamManagement（子账号 + 配额分配） | `04dbe7b` |
| 3 | 新增子账号创建 + team quota 分配端点 + quota repository（pool/allocation/ledger 三层账） | `62f513c` |
| 4 | 平台租户（`deeptrols-platform`）配额池：系统管理员可创建/分配池 | 平台 bootstrap |

### 9.2 配额池 CRUD 修复

| # | 现象 | 根因 | 修复 |
|---|------|------|------|
| 1 | 配额池列表 500 | 租户级池 `model_id` 为 NULL，pgx 扫描 NULL UUID→string 报错 | `quotas.go` 查询用 `COALESCE(qp.model_id::text,'')` |
| 2 | 创建对话框要求手填租户 UUID | 前端把 tenant_id 当自由输入 | `QuotaManagement.tsx` 改为租户下拉，未选租户禁用提交；`total_amount`/`amount` 加 `min={1}` |
| 3 | 分配重试会重复发配额 | 幂等键每次请求新 UUID | 幂等键改为确定性 `(pool,user,amount)`（`quotas.go` / `team_quotas.go`），重试回放同一条分配记录 |
| 4 | 非法 model_id/tenant_id 落到 500 | handler 未在入口校验 | create 前校验 UUID（400 早退）；配额账簿 `allocation_id` 校验 |

### 9.3 新增测试（console 包）

- `TestHandleListQuotaPools_WithNullModel` — NULL model_id 列表不 500
- `TestHandleCreateQuotaPool_RequiresTenantID` / `_RequiresTenantID_EmptyString` / `_InvalidModelID` / `_InvalidTenantID` — 输入校验 400
- `TestHandleAllocateQuota_RetryIsIdempotent` — 重试分配只入账一次

### 9.4 验证

- `go build ./...` · `go vet ./...` ✅
- console handler + quota repository 测试全绿
- `npx tsc --noEmit` ✅
- 浏览器 E2E：列表加载 3 个池无 500；创建「新龙科技 1.0M」池后 3→4；创建对话框租户必选、分配成功
- 提交 `a3c2baa`（含本重构相关 3 提交），推送 origin/main

### 9.5 状态更新

- 配额管理（控制台）已由「只读」升级为「创建 + 分配」（对应功能清单第八篇）
- AI 配额池（三层账 + 幂等）✅ 已实现（对应功能清单第五篇，原 ❌）

### 9.6 企业管理：终止 → 真实删除

**背景**：企业管理（`Tenants.tsx`）原「终止」按钮实际是软终止——`DELETE /api/admin/tenants/{id}` 只把 `status` 置为 `terminated`，租户行仍留在列表（用户反馈「界面还是有数据」）。

**变更**（本次 commit）：
- `tenant.Repository` 新增 `Delete(ctx, id)`（返回包级哨兵 `tenant.ErrNotFound`，对齐 wallet/quota 惯例）
- `PostgresRepository.Delete` 单事务级联硬删：`quota_ledger → quota_allocations → quota_pools → tenant_models → tenants`（叶子先删，规避 RESTRICT）；`tenant_memberships`/`tenant_invitations` 由 ON DELETE CASCADE 自动清理；`api_keys/wallets/usage_logs/channels/route_policies/model_pricing/audit_logs` 的裸 `tenant_id` 无 FK，**保留不动**以保全计费/用量证据
- `HandleDeleteTenant` 由状态翻转改为真删除：保持 admin 校验（401）/404/平台租户保护（403）；删除前将租户 code/name 写入操作日志（审计中间件只记 UUID，硬删除后无法还原身份）
- 前端：`terminate` → `delete`，按钮改「删除」，**所有状态行**（含 terminated/rejected/pending_review）均可删除；删除确认弹窗隐藏原因输入、文案改为「删除后该企业及其成员、配额、模型配置将被永久移除，此操作不可撤销。」，确认按钮为「确认删除」
- 测试：`TestHandleDeleteTenant_HardDelete`（行消失）、`TestHandleDeleteTenant_CascadeCleanup`（成员/模型/配额三级依赖全清 + 全局模型保留）、仓库层 `Delete`/`ErrNotFound` 子测试

**验证**：`go build ./...` · gofmt clean · `go test -p 1 ./internal/repository/tenant/ ./internal/handler/console/` 全绿 · `npx tsc --noEmit` ✅

**安全评审备注（遗留建议，非本次阻塞）**：`/api/admin` 路由组整体无速率限制（登录/网关/团队均有，仅 admin 组缺失），建议后续加 admin 限流；审计中间件 `recordAudit` 不落 `old_value`，硬删除审计建议后续扩展为记录被删租户身份。

**上线运维要点**：`DELETE /api/admin/tenants/{id}` 路由随本次 commit 才注册，**必须重启 API 进程**才能生效——旧二进制会 404，前端 `catch {}` 吞错、列表不刷新，表现为「点击删除界面仍有显示」。已实测重启后：登录 → 建临时租户 → DELETE 200 `{"status":"deleted"}` → 列表出现次数 0。

**企业域名清理（已处理）**：确认不需要企业域名后，将 `tenant_domains` 从线上库彻底 DROP（应用现成迁移 000007，`migrate up`），删除 RESTRICT 外键隐患；同步移除 `internal/domain/tenant.go` 中无引用的死代码 `TenantDomain` 结构体。清理中发现线上库 `schema_migrations` 仅记录到 v6（000007/000008 均未登记）：000008 的唯一约束 `quota_allocations_pool_user_unique` 实际已存在于库中，迁移失败置 dirty，遂 `migrate force 8` 对齐版本（8|f）。后续新迁移可直接 `migrate up` 正常执行。

### 9.7 配额池：编辑 + 删除（补全 CRUD）

**背景**：配额管理页面只有「创建 + 分配」，缺修改与删除（用户反馈「配额管理的CURD有问题，没有删除修改的功能」）。后端只有 `GET/POST /quotas` 与 `POST /quotas/{id}/allocate`，无 `PUT/DELETE` 路由。

**变更**（本次 commit）：
- `quota.Repository` 新增 `UpdatePool` / `DeletePool` + 哨兵 `ErrConstraintViolation`
- `PostgresRepository.UpdatePool` 事务内 `SELECT ... FOR UPDATE` 锁定池行，拒绝 `total_amount < allocated_amount`（防 TOCTOU 超卖）；可编辑字段仅 `total_amount/unit_name/dimension`，作用域（tenant/model）不可变
- `PostgresRepository.DeletePool` 单事务叶子先删级联：`quota_ledger → quota_allocations → quota_pools`（对齐租户硬删除模式）
- 新增 handler `HandleUpdateQuotaPool` / `HandleDeleteQuotaPool`，注册 `PUT/DELETE /quotas/{id}`
- **DELETE 返回 200 + `{"status":"deleted"}` 而非 204**：前端 `request()` 恒读 `res.json()`，204 无 body 会让成功删除被误报为错误（实测复现 → 修复，对齐 `HandleDeleteTenant` 惯例）
- 前端 `QuotaManagement.tsx`：操作列新增「编辑」「删除」按钮 + 编辑对话框（预填、新总量不得低于已分配提示）+ 删除确认弹窗（级联警示文案）；约束拒绝（400）经 `toast.error` 透出，不静默失败
- 修复既有测试包 mock 未满足增长后的 `quota.Repository` 接口（billing `mockQuotaRepo` / gateway `mockQuotaRepoForChat` 补 7 个 stub）

**验证**：`go build ./...` · `go vet ./...` · gofmt clean；repo/handler/service/gateway 四个包测试全绿；前端 `tsc --noEmit` + vitest 225 例全过；浏览器 E2E：编辑 5.0M→6.0M 刷新保留、新总量低于已分配返回 400 带 toast、删除 3 池→1 池级联落库、列表逐次刷新。

### 9.8 企业自助注册 + 个人/企业登录切换

**背景**：登录/注册区分个人与企业账号。企业账号由企业自行注册，进入 `pending_review` 待平台管理员审核；个人走原有注册。登录表单不变，仅切换文案与注册链接目标。

**变更**（本次 6 commits：`cba171d` `f67328e` `192802d` `ec58748` `6bd97a9`）：
- 后端 `POST /api/console/auth/register/enterprise`：单事务创建 用户(enterprise/active) + 租户(pending_review) + owner 成员 + 零余额钱包；`deriveTenantCode` 防租户 code 碰撞；成功后写 auth cookie 自动登录；`/me` 返回 `tenant_status`
- 前端登录页（`Login.tsx`）个人/企业分段切换：仅换文案 + 注册链接（`/register?type=personal|enterprise`），表单不变；注册页（`Register.tsx`）双表单：个人(昵称/邮箱/密码) / 企业(公司名/联系人/邮箱/密码)，`?type=` 初始化、切换清空已填字段，企业提交走 `registerEnterprise`
- `PendingReviewBanner`：`tenant_status=pending_review` 时控制台各页顶部显示琥珀色「企业账号审核中」提示（`role="status"` 可读屏播报）；`auth.tsx` 新增 `AuthUser.tenant_status` / `registerEnterprise`
- **安全加固**（security-reviewer BLOCK → 修复）：注册端点按 IP 限流（5/分钟）、`HandleRegisterEnterprise` 请求体限 8KB + company/contact 上限 255、前端 `register` 对 4xx/5xx 不再静默成功（对齐「错误不能伪装成成功」不变量）

**验证**：`go build/vet` clean · `go test -p 1 ./internal/handler/console/` 全绿 · 前端 `tsc --noEmit` + vitest 237 例全过；code-reviewer APPROVE（0 CRITICAL/0 HIGH）、security-reviewer 遗留项见下。

**安全评审遗留（follow-up，非本次阻塞）**：① 默认管理员密码 `deeptrols@2026` 为预置 infra（`internal/config/config.go`），建议启动时强制显式提供；② 密码策略仅最小 8 位，无复杂度要求（与既有个人注册一致），复杂度为产品策略待定；③ `ConsoleAuth`/`GatewayAuth` 不校验租户状态，`pending_review` 企业可进控制台但钱包零余额无法计费调用——是否在中间件层硬拦截待产品决策；④ 注册 409「Email already registered」暴露邮箱占用（标准注册 UX，与既有注册一致，已随限流缓解枚举放大）。


## 十、2026-08-13 砍掉「配额」概念：企业管理员充值 → 给员工分配余额

> **背景**：企业管理员（`demo@admin.com`）在团队管理分配额度时报「企业暂无配额池，请联系平台管理员创建后再分配」；企业员工（`demo@demo.com`）在线体验报 `Insufficient balance`。与用户对齐后**放弃「配额池」三层账务模型**（pool/allocation/ledger），改为最简钱包模型：**企业管理员充值（钱进自己钱包）→ 给员工分配余额（钱包转账）→ 员工用余额调用模型**。网关本就无条件扣钱包，员工钱包有余额即零改动 work。

### 10.1 四角色权限矩阵（目标态）

| 角色 | Admin 控制台 | 团队管理 | 钱包管理 | 余额/额度 |
|---|---|---|---|---|
| 系统管理员 `role=admin` | ✅（去掉「配额管理」菜单） | ✅ | ✅ | — |
| 个人用户 | ❌ | ❌ | ✅ | 自充自用 |
| 企业管理员 `tenant_role=owner/admin` | ❌ | ✅ | ✅ | 充值 + 给员工分配余额 |
| 企业员工 `tenant_role=member` | ❌ | ❌ | ❌（隐藏） | 余额由管理员分配，只读 |

### 10.2 后端变更（commit `fcffdc6`）

| # | 变更 | 文件 |
|---|------|------|
| 1 | `wallet.Repository` 新增 `Transfer(ctx, fromWalletID, toWalletID, amount, idempotencyKey)`：单事务 FOR UPDATE 锁双钱包、余额校验、确定性幂等键、两条流水（`transfer_out`/`transfer_in`，`reference_type='balance_transfer'`） | `internal/repository/wallet/repository.go` / `postgres.go` |
| 2 | 新增 `HandleAllocateBalance`：`POST /team/balance/allocate`，复用 `isTenantAdmin` 鉴权；请求 `{user_id, amount}`（金额 decimal 字符串）；**强制同租户转账**（目标 `tenant_id` 不匹配 403/404）；`amount<=0` 400；余额不足 400「钱包余额不足」 | `internal/handler/console/team_balance.go` |
| 3 | 路由注册（`/team` 限流组内） | `cmd/api/main.go` |
| 4 | **安全评审 H1 修复（follow-up commit）**：幂等键纳入金额 → `"balance-transfer:" + adminUserID + ":" + targetUserID + ":" + amount`，同成员**不同金额**是独立转账，不再静默回放首笔（否则第二次分配 200 OK 但钱不动）；`Transfer` 重放路径校验存储金额一致，不一致返回新哨兵 `ErrIdempotencyMismatch`（handler → 409）；锁双钱包改**规范顺序**（按 id 排序）防 A→B / B→A 并发死锁；目标钱包 UPDATE 补 `RowsAffected` 乐观锁检查；`team/balance/allocate` 加 `http.MaxBytesReader` 8KB 体限制 | `internal/repository/wallet/postgres.go` / `repository.go` / `team_balance.go` |

### 10.3 前端变更

| 文件 | 变更 |
|------|------|
| `web/src/pages/TeamManagement.tsx` | 「分配额度」对话框 → 「分配余额」：输入 CNY 金额（`toMoneyInput` 清洗 + 正则 `/^\d+(\.\d{1,2})?$/` + 上限 = 管理员 `/wallet` 的 `available`）；成员列表新增「余额」列（decimal 字符串经 `fmtMoney` 展示）；mutation 指向 `/team/balance/allocate`，成功同时失效 `/team` + `/wallet` |
| `web/src/components/ConsoleLayout.tsx` | 「钱包管理」菜单对企业员工（`tenant_role === "member"`）隐藏（前端只读 + 后端不开放员工充值为加固项） |
| `web/src/components/AdminLayout.tsx` | 删除「配额管理」菜单项 |
| `web/src/App.tsx` | 删除 `/admin/quotas` 路由（`QuotaManagement.tsx` 保留但不可达） |

### 10.4 测试

- `internal/repository/wallet/postgres_test.go`：`Transfer` 集成测试（成功转账双方余额变化 + 双流水；余额不足 `ErrInsufficientBalance`；幂等重放不重复扣款；目标钱包不存在 `ErrNotFound`；**新增**：同一 key 复用于不同金额 → `ErrIdempotencyMismatch`，且被拒转账不移动资金）
- `internal/handler/console/team_balance_test.go`：非租户管理员 403；跨租户目标 403/404；`amount<=0` 400；余额不足 400；**新增** `TestHandleAllocateBalance_DifferentAmountsToSameMember`（H1 回归：同成员 10+20 两次分配均成功、交易号不同、余额 70/30；同金额重试回放不重复扣款）
- 前端：`TeamManagement.test.tsx`（分配余额对话框 + `/team/balance/allocate` 提交 + 余额列展示 + 超上限禁用 + **wallet 加载失败禁用分配并提示重试**）、新增 `ConsoleLayout.test.tsx`（四角色导航：企业员工无钱包管理/团队管理/管理控制台，企业管理员有钱包+团队，个人用户有钱包无团队，系统管理员有钱包+管理控制台）、新增 `money.test.ts`（`toCents`/`fmtMoney`/`toMoneyInput`/`isValidAmount`，BigInt 超越 float 安全整数精度）

### 10.5 验证

- `go build ./...` · `go vet ./...` · `go test ./...` 全绿（含 `internal/repository/wallet` + `internal/handler/console`）
- 前端 `npx tsc --noEmit` · vitest（27 文件 / 250 例）· `npm run build` 全绿

### 10.6 边界 / 安全

- 转账**强制同租户**（后端校验 `tenant_id` 匹配），禁止跨租户划转
- 金额全程 decimal 字符串穿越 API 边界，禁止 float；FOR UPDATE 防并发超扣；确定性幂等键**含金额**（`balance-transfer:<admin>:<member>:<amount>`）——同成员不同金额是独立转账，同金额重试回放不重复扣款；重放金额与存储不一致返回 `ErrIdempotencyMismatch`（handler → 409），绝不伪装成功；双钱包锁按 id 规范顺序获取防死锁
- 员工「无钱包管理」= 前端隐藏 + `/wallet/topup` 未对企业员工开放（管理员分配是唯一入账路径）
- 预算控制语义保留：员工余额上限 = 管理员分配给他的余额，花完即止（网关 402 `insufficient_balance`，提示管理员再分配）

## 十一、2026-08-13 修复：管理控制台「个人管理」删除后仍显示

> **Bug**：系统管理员在「管理控制台 → 个人管理」删除一个用户后，前端列表仍显示该用户。根因是 `HandleDeleteUser` 是**软删除**（`status='deleted'`，保留记录供审计/证据链），而 `HandleListUsers` 的 SQL 不过滤该状态，把已删除用户原样返回，前端 refetch 后依然可见。

### 11.1 变更

| # | 变更 | 文件 |
|---|------|------|
| 1 | `user.ListFilter` 新增 `ExcludeDeleted bool`；**零值保持向后兼容**（审计/证据路径继续查全量） | `internal/repository/user/repository.go` |
| 2 | `List`/`Count` 的 WHERE 改为 conditions-join：`UserType` 与 `status <> 'deleted'` 用 `AND` 组合，参数编号正确 | `internal/repository/user/postgres.go` |
| 3 | `HandleListUsers` 设 `ListFilter{ExcludeDeleted: true}` —— 管理列表不再返回已删除用户，mutation 后前端 invalidation refetch 即消失 | `internal/handler/console/users.go` |

### 11.2 测试（TDD，先 RED 后 GREEN）

- `internal/repository/user/postgres_test.go`：`TestListExcludesDeleted` —— 零值 filter 含已删除用户（向后兼容）；`ExcludeDeleted` 时 `List`/`Count` 均排除。
- `internal/handler/console/users_test.go`：`TestHandleListUsers_ExcludesDeleted` —— 删一人后 `?user_type=personal` total=2（admin+survivor），victim 不出现在响应。

### 11.3 验证

- `go build ./...` · `go vet ./...` 全绿；`go test ./internal/repository/user ./internal/handler/console` 全绿（console 包 191s 全量通过）。
- 前端 `Users.test.tsx` 5/5 通过（前端无改动——invalidation 机制本就会 refetch，修复落在后端列表）。

## 十二、2026-08-18 计费正确性收尾（生产就绪 Step 1）

> **目标**：修复计费/证据面三个 Blockers——流式 usage 来源标记缺失（不变量 #4）、流式错误伪装成功（不变量 #5）、价格快照无证据。

### 12.1 变更

| # | 变更 | 文件 |
|---|------|------|
| 1 | 迁移 000009：`model_pricing.price_version`（默认 1）；`usage_logs.usage_source` CHECK 放行 `cached`（此前缓存命中落库必被拒） | `migrations/000009_billing_evidence.up/down.sql` |
| 2 | `ModelPricing.PriceVersion` 字段；`FindByModel` 读取 `price_version` | `internal/domain/model.go`、`internal/repository/model/postgres.go` |
| 3 | 定价变更递增版本：`HandleSetMarkup` 更新时 `price_version = price_version + 1` | `internal/handler/console/pricing.go` |
| 4 | Pricer：charge line 携带 `PriceSource=model_pricing` + `PriceVersion`；`PriceSnapshot` 填充 source/currency/captured_at/rows（decimal 一律字符串），无定价行时 rows 为空数组 | `internal/service/billing/pricer.go` |
| 5 | Logger：charge line 优先 per-line `PriceSource/PriceVersion`，为空回退 params 级 | `internal/service/billing/logger.go` |
| 6 | 网关流式：最终 chunk usage 标记 `final_chunk`；新增 `logStreamFailure`（detached context，partial/failed，错误码分类：upstream_error / upstream_http_error / streaming_not_supported / stream_interrupted / client_disconnected；证据含上游状态码与错误体（上限 1 MiB）、请求体与 tenant_id；成本与钱包扣费恒为 0）；`client.Do` 错误 / 上游 ≥400 / flusher 不支持 / scanner 错误四类失败路径全部落日志，scanner 错误路径释放预留金也走 detached context | `internal/handler/gateway/chat.go` |

### 12.2 不变量状态

| # | 不变量 | 状态 |
|---|--------|------|
| 4 | usage 来源显式标记 | ✅ 补齐 `final_chunk`；`cached` 可落库 |
| 5 | 流式错误不伪装成功 | ✅ 失败/截断路径不再落 `completed`；scanner 错误不发送 [DONE] 且落 `partial`/`failed` |

### 12.3 测试（TDD，先 RED 后 GREEN）

- 网关：`TestHandleStreamingChat_SuccessWithUsage`（期望 `final_chunk` + completed + 证据）；新增 `TruncatedStream_LogsPartialAndReleases`（RST 截断：无 [DONE]、partial、stream_interrupted、零扣费）、`UpstreamHTTPError_LogsFailed`（failed + evidence 500）、`UpstreamConnectionError_LogsFailed`
- pricer：快照填充/来源版本/无定价空 rows；logger：per-line 来源版本优先 + params 回退
- 集成：`TestFindByModel_ScansPriceVersion`、`TestCreateUsageLog_CachedSource`

### 12.4 验证

- `go build ./...` · `go vet ./...` 全绿；`go test -p 1 ./...` 全量通过（串行避免共享测试库并发 TRUNCATE 死锁）
- 迁移 000009 已应用 dev + test 库；up/down 往返验证通过

## 十三、2026-08-18 生产就绪 Step 2/3：非流式证据 · 安全基线 · 网关端点扩展

> **目标**：按生产就绪路线补齐三件事——非流式失败请求不再"账外消失"、管理后台限流与生产配置 fail-fast、网关新增 embeddings/images/audio 端点走完整计费链路。

### 13.1 非流式失败证据补齐（对账"消失的请求"清零）

`POST /v1/chat/completions`（stream=false）上游全部失败时，此前只 release 预留金并返回 502，**不写 usage_log**。现在：

| # | 变更 | 文件 |
|---|------|------|
| 1 | 候选循环内记录最后一次尝试（statusCode / duration / response body / attemptCount），供最终失败日志使用 | `internal/handler/gateway/chat.go` |
| 2 | 新增 `logNonStreamFailure`：detached context（30s，客户端断连不丢证据）；`Status=failed`、`UsageSource=estimated`、成本/钱包/配额恒 0；错误码分类 `upstream_http_error`（带 status_code）/ `upstream_error` / `client_disconnected`；evidence 带上游状态码、错误体（1 MiB 截断）、请求体、tenant_id；`(all N candidates failed)` 可追溯多候选失败 | `internal/handler/gateway/chat.go` |
| 3 | `releaseHoldSafe`：客户端断开（ctx canceled）时改用 detached context 释放预留金与配额，防断连冻结 | `internal/handler/gateway/chat.go` |

### 13.2 管理后台限流 + 生产配置基线 + 审计 old_value

| # | 变更 | 文件 |
|---|------|------|
| 1 | 新增 `AdminRateLimit`（key=`rl:admin:<userID>`，IP 兜底，fail-open），挂到 `/api/admin` 组（AdminAuth 之后、AuditAdminWrite 之前），默认 120/min | `internal/handler/middleware/ratelimit.go`、`cmd/api/main.go` |
| 2 | 生产模式（`ENABLE_FAKE_PAYMENT=false`）强制 `COOKIE_SECURE=true`、`ADMIN_PASSWORD` ≥ 12 字节，否则拒绝启动（fail-fast） | `internal/config/config.go` |
| 3 | 审计落库支持 `old_value`：`CtxAuditOldValue` 上下文快照 → `audit_logs.old_value` JSON；租户硬删除与用户软删除前写入身份快照（id/code/name/email/role 等），删除后可追溯 | `internal/handler/middleware/audit.go`、`internal/handler/console/tenants.go`、`internal/handler/console/users.go` |
| 4 | `.env.example` 补生产环境注释块（强随机密钥生成、TLS 终止、COOKIE 基线）；新增 `docs/DEPLOYMENT.md` 部署手册（环境变量基线、迁移与 dirty 修复、健康检查、备份、密钥轮换、灰度回滚） | `.env.example`、`docs/DEPLOYMENT.md` |

### 13.3 网关端点扩展：/v1/embeddings · /v1/images/generations · /v1/audio/speech

| # | 变更 | 文件 |
|---|------|------|
| 1 | Executor 接口新增 `ExecuteEndpoint` / `ExecuteEndpointRaw`（任意 `/v1` 端点转发；`Execute` 改为委托 `chat/completions`，行为不变） | `internal/service/gateway/executor.go` |
| 2 | 新增通用转发助手 `handleForwardedEndpoint` / `handleForwardedRawEndpoint`：POST+1MiB 校验 → `enforceAPIKeyBoundaries` → 路由候选（3 个 failover）→ **预留金与配额预留先于上游调用** → 逐候选执行（失败 release 换下一个）→ 成功后估算/解析 usage → pricer 计价 → Settle → spend 记录 → usage_log（request_type 参数化） | `internal/handler/gateway/endpoints.go` |
| 3 | `HandleEmbeddings`：usage 优先响应 `usage.prompt_tokens`（source=upstream），缺失按输入估算（source=estimated）；request_type=embeddings，按 input 维度计价 | `internal/handler/gateway/endpoints.go` |
| 4 | `HandleImagesGenerations`：按请求 `n`（默认 1，>10 拒绝）→ `ImageCount`，dimension=image；`HandleAudioSpeech`：输入文本估算 TTS 字符，dimension=tts；audio/speech 走 Raw 响应透传（二进制音频） | `internal/handler/gateway/endpoints.go` |
| 5 | 路由注册：`r.Post("/embeddings" ...)` 等三个路由挂入 /v1 组 | `cmd/api/main.go` |

### 13.4 测试（TDD，先 RED 后 GREEN）

- 非流式失败：HTTP 500 → failed + upstream_http_error + evidence.StatusCode=500 + release；连接错误 → upstream_error；多候选全失败 → 单条日志含 attempts；客户端取消 → release 仍执行、日志仍落；成功路径无 failed 日志（回归）
- 限流：`TestAdminRateLimit_*` 同用户超限 429 + Retry-After、不同用户互不影响、无用户按 IP 兜底
- 配置：生产模式 COOKIE_SECURE=false / 弱管理员密码 → 启动报错；开发模式默认值放行（回归）
- 审计：`audit_logs.old_value` 落 JSON 快照（集成）
- 端点：embeddings/images/audio 各自成功结算（dimension=input/image/tts、request_type 正确、usage 来源 upstream/estimated 兜底）、上游 4xx 失败落证据零成本、无钱包 402、未知模型 404、n>10 拒绝

### 13.5 验证

- `go vet ./...` · `go build ./...` 全绿；`go test -p 1 ./...` 全量通过
- **注记（测试基建）**：共享测试库曾受一个被中断运行的孤儿测试进程干扰（表现为随机 deadlock / FK 违规 / 计数错乱），已用 `pg_terminate_backend` 清掉其连接后全量复测通过；复现排查手法记录在 `docs/DEPLOYMENT.md` 与本次会话，非代码缺陷
- 新增端点已注册；API 二进制重建重启后 /health 正常

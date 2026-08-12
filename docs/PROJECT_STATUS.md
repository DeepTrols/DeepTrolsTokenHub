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

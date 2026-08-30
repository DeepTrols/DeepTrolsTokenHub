# DeepTrols — AI Token 聚合平台

企业级 AI 模型推理聚合平台。不只是反向代理——围绕模型调用构建完整的计费、风控、对账与运营系统。

## 架构

```
控制面 (Control)  → API Key / JWT / 租户隔离 / 模型目录 / 渠道凭证
执行面 (Execution) → OpenAI 兼容直连 / Provider Adapter / 加权路由 / Fallback / 响应缓存
资金面 (Money)    → Reserve→Commit→Release / 钱包 / Price Snapshot
证据面 (Evidence) → Usage Log / Charge Line / Provider Evidence / 对账 L0+L1
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.22 + chi v5 + pgx v5 + go-redis v9 |
| 数据库 | PostgreSQL 16 |
| 缓存 | Redis 7 |
| 执行代理 | OpenAI 兼容直连（渠道实例 base_url；内置 LiteLLM 已于 2026-08-19 移除） |
| 前端 | React 18 + TypeScript + Vite + Tailwind CSS + shadcn/ui + TanStack Query |

## 项目结构

```
cmd/api/main.go              # API 进程入口
cmd/worker/main.go           # Worker 进程入口
internal/
  app/                       # App 结构体（依赖注入 + 路由装配）
  config/                    # 集中配置管理（环境变量 → Config 结构体）
  domain/                    # 领域模型
    apikey.go                #   APIKey（含 6 边界治理）
    auth.go                  #   Auth（登录/注册/Token 请求体）
    channel.go               #   Channel + Instance（渠道实例）
    model.go                 #   Model（含多维定价）
    tenant.go                #   Tenant（5 状态状态机）
    usage.go                 #   UsageLog + ChargeLine + ProviderEvidence
    user.go                  #   User（角色 + 状态）
    wallet.go                #   Wallet（乐观锁版本号）
  handler/
    gateway/                 # OpenAI-compatible 网关（/v1/chat/completions + /v1/models + embeddings/images/audio 转发）
    console/                 # 控制台 API（认证/密钥/用量/钱包/模型/团队/租户）
    middleware/              # 鉴权/租户/限流/安全头/CORS
  service/
    billing/                 # 计费引擎（Reserve/Commit/Release + 定价）
    cache/                   # 响应缓存（SHA256→Redis，命中零计费）
    gateway/                 # 网关服务（路由 + OpenAI 兼容直连执行）
  repository/                # 数据访问接口（PostgreSQL 实现）
    apikey/ channel/ model/ tenant/ usage/ user/ wallet/
  worker/
    health_checker/          # 渠道实例健康探测（60s 周期，渐进评分）
    reconciliation/          # 对账 Worker（L0 漏记账 + L1 证据不匹配）
  pkg/
    db/                      # pgxpool 封装 + 事务工具
    decimal/                 # shopspring/decimal 辅助
    encrypt/                 # AES-256-GCM 加密（API Key 加密存储）
    idempotency/             # 幂等键管理
    jsonb/                   # PostgreSQL JSONB 辅助
    jwtutil/                 # JWT HS256 签发/验证
    keyhash/                 # HMAC-SHA256 API Key 哈希
    lease/                   # Redis 分布式选主（SET NX EX）
    ratelimit/               # 限流（Redis 优先 + 内存降级）
    redis/                   # go-redis 客户端封装
    usageparser/             # 上游 usage 解析（OpenAI 兼容格式）
migrations/                  # PostgreSQL DDL（9 次迁移 000001~000009，24+ 张表）
web/                         # React 前端（21 页面）
```

## 快速启动

### 1. 前置条件

- Go 1.22+
- Docker & Docker Compose
- Node.js 18+（前端开发）
- [golang-migrate](https://github.com/golang-migrate/migrate)（数据库迁移）

### 2. 一键启动（Docker Compose）

```bash
docker compose up -d
```

启动 5 个容器：`api` + `worker` + `web` + `postgres` + `redis`。首次启动后自动创建管理员账户。

### 3. 本地开发启动

```bash
# 1. 启动基础设施
docker compose up -d postgres redis

# 2. 配置环境变量
cp .env.example .env
# 编辑 .env，本地开发默认值可直接使用

# 3. 数据库迁移
migrate -path migrations -database "$DATABASE_URL" up

# 4. 启动后端
go run ./cmd/api          # API 服务器（:8080）
go run ./cmd/worker       # Worker（后台任务）

# 5. 启动前端
cd web && npm install && npm run dev   # Vite 开发服务器（:5173）
```

### 4. 关键环境变量

| 变量 | 说明 | 开发默认值 |
|------|------|-----------|
| `DATABASE_URL` | PostgreSQL 连接串 | `postgresql://deeptrols:deeptrols_dev@localhost:5432/deeptrols` |
| `REDIS_URL` | Redis 连接串 | `redis://localhost:6379/0` |
| `JWT_SECRET` | JWT 签名密钥 | 至少 32 字节 |
| `ENCRYPTION_KEY` | AES-256 加密密钥 | 必须正好 32 字节 |
| `CORS_ORIGIN` | 前端域名 | `http://localhost:5173` |
| `COOKIE_SECURE` | Cookie Secure 标记 | 开发 `false`，生产 `true` |
| `API_PORT` | API 监听端口 | `8080` |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | 自动创建的管理员 | `deeptrols@admin.com` / `deeptrols@2026` |
| `ENABLE_FAKE_PAYMENT` | **演示充值开关（资金安全）** | **`false`（生产必须保持）** |

> ⚠️ **`ENABLE_FAKE_PAYMENT`**：`false`（默认）时 `/wallet/topup` 返回 403，注册与管理员不再自动赠送余额；同时 API 启动时**拒绝已知弱密钥**（JWT/加密/管理员密码命中默认值时直接退出）。开发环境需要造币口时设 `true`。
>
> ⚠️ **生产密钥**：docker-compose 模板中的默认值均为 DEV-ONLY。生产部署请 `cp .env.example .env` 后设置强随机密钥（`openssl rand -hex 32` / `openssl rand -hex 16`），`ENABLE_FAKE_PAYMENT` 保持 `false`。

## 默认账户

| 角色 | 邮箱 | 密码 |
|------|------|------|
| 管理员 | `deeptrols@admin.com` | `deeptrols@2026` |

首次登录后建议立即修改密码。管理员拥有模型管理、Provider 凭证、Channel、租户、对账、用户管理等全部后台权限。

## API 端点

### Gateway（OpenAI 兼容，API Key 鉴权）

| 端点 | 说明 |
|------|------|
| `POST /v1/chat/completions` | 聊天补全（流式 SSE + 非流式，含响应缓存） |
| `GET /v1/models` | 可用模型列表（按 API Key allowlist 过滤） |
| `POST /v1/embeddings` | 嵌入向量（转发 + 计费闭环） |
| `POST /v1/images/generations` | 图片生成（转发 + 按图计费） |
| `POST /v1/audio/speech` | 文字转语音（raw 转发 + TTS 字符计费） |

### Console（用户端，Cookie 鉴权）

| 端点 | 说明 |
|------|------|
| `POST /api/console/auth/login` | 登录 |
| `POST /api/console/auth/logout` | 登出（服务端清除 cookie） |
| `POST /api/console/auth/register` | 注册 |
| `GET /api/console/me` | 当前用户信息 |
| `PUT /api/console/me/password` | 修改密码 |
| `GET/POST /api/console/api-keys` | API 密钥管理（6 边界 CRUD） |
| `GET /api/console/api-keys/{id}/secret` | 查看 API Key 明文（仅一次） |
| `GET /api/console/usage` | 调用日志（含费用明细） |
| `GET /api/console/wallet` | 钱包余额 + 交易记录 |
| `POST /api/console/wallet/topup` | 在线充值 |
| `GET /api/console/models` | 模型广场（含定价） |
| `GET /api/console/security/login-history` | 登录历史 |

### Admin（管理端，Cookie + admin 角色）

| 端点 | 说明 |
|------|------|
| `CRUD /api/admin/models` | 模型管理（Provider 下拉 + 9 维定价） |
| `CRUD /api/admin/providers` | Provider 凭证管理（14 家默认 URL + 加密存储） |
| `POST /api/admin/providers/{id}/sync` | 从上游 API 同步模型 |
| `CRUD /api/admin/channels` | Channel 管理（模型绑定 + 实例增删） |
| `CRUD /api/admin/tenants` | 租户管理（5 状态状态机 + 域名） |
| `GET /api/admin/reconciliation` | 对账结果查看 |
| `GET/POST/PUT/DELETE /api/admin/users` | 用户管理（CRUD + 角色/状态） |
| `GET /api/admin/ledger` | 账号账簿（账务管理） |

## 前端页面

### 用户端（11 页面）

| 页面 | 路由 | 说明 |
|------|------|------|
| 登录 | `/login` | 邮箱 + 密码 |
| 注册 | `/register` | 新用户注册 |
| 用量信息 | `/dashboard` | 用量概览 + 关键指标 |
| API 密钥 | `/api-keys` | 密钥管理（CRUD + 6 边界） |
| 调用记录 | `/logs` | 用量历史 + 费用明细 |
| 用量统计 | `/usage` | 用量图表 + 统计 |
| 充值 | `/recharge` | 在线充值（演示造币口） |
| 账单 | `/bills` | 充值/交易记录 |
| 模型广场 | `/models` | 模型浏览 + 定价 |
| 在线体验 | `/playground` | 模型在线测试 |
| 用户中心 | `/account` | 账户资料 + 团队管理 + 登录历史 |
| 开发文档 | `/docs` | API 使用指南 |

### 管理端（6 页面）

| 页面 | 路由 | 说明 |
|------|------|------|
| 模型管理 | `/admin/models` | 模型 CRUD + 多维定价 |
| 渠道管理 | `/admin/channels` | Provider 凭证 CRUD（创建时自动发现模型并建渠道） |
| 对账管理 | `/admin/reconciliation` | 对账运行记录 |
| 企业管理 | `/admin/tenants` | 租户 CRUD + 审核/状态 |
| 个人管理 | `/admin/users` | 用户 CRUD + 角色/状态 |
| 账务管理 | `/admin/finance` | 账号账簿 |

## 使用示例

### curl 调用（Cookie 方式）

```bash
# 1. 登录并保存 cookie
curl -c /tmp/cookies.txt -X POST http://localhost:8080/api/console/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"deeptrols@admin.com","password":"deeptrols@2026"}'

# 2. 创建 API 密钥（自动携带 cookie）
curl -b /tmp/cookies.txt -X POST http://localhost:8080/api/console/api-keys \
  -H "Content-Type: application/json" \
  -d '{"name":"my-key"}'

# 3. 登出
curl -b /tmp/cookies.txt -X POST http://localhost:8080/api/console/auth/logout
```

### 调用模型（API Key 方式）

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer sk-<your-key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}'
```

## 核心特性

### 计费引擎

- **Reserve-Commit-Release**：上游调用前锁预算，成功提交，失败释放
- **乐观锁**：`wallet.version` 字段防并发扣款
- **幂等保护**：`idempotency_key` 唯一约束防重复扣费
- **9 维定价**：input / output / cache_read / cache_write / reasoning / image / audio / tts / video
- **decimal 精度**：所有金额计算使用 `shopspring/decimal`，禁止浮点数
- **三表事务写入**：`usage_log` + `charge_line` + `provider_evidence` 单事务落库
- **流式计费闭环**：SSE 最后 chunk 提取 usage → detached context 异步提交
- **计费同步化**：Reserve → Settle（按真实用量多退少补）→ Release 单请求内完成，无需异步对账补偿

### 响应缓存

- **SHA256(request) → Redis**：相同请求命中缓存，零计费
- **X-Cache: HIT / MISS**：透明告知缓存状态

### 安全

- **API Key**：HMAC-SHA256 哈希查表，6 边界治理（模型白名单 / IP / 累计 / 周限额 / 月限额 / 超限动作）
- **JWT 用户鉴权**：HS256，httpOnly cookie + SameSite Strict，服务端 logout
- **租户隔离**：Host header → tenant_domains 表匹配
- **限流**：Redis 优先 + 内存降级，网关按 Key 限流，登录按 IP 限流
- **安全头**：CSP / HSTS / X-Frame-Options / X-Content-Type-Options / Referrer-Policy
- **加密存储**：API Key 明文 AES-256-GCM 加密，`ENCRYPTION_KEY` 泄露 = 所有 Key 暴露

### Worker 后台任务

| Worker | 周期 | 说明 |
|--------|------|------|
| Health Checker | 60s | 探测所有渠道实例 /health 端点 |
| Reconciler | 1h | L0（漏记账检查）+ L1（证据不匹配检查） |

## 5 个不变量

| # | 不变量 | 实现方式 |
|---|--------|---------|
| 1 | `request_id` 不是全局唯一账务身份 | 复合身份 `tenant+user+key+type+request_id` |
| 2 | 预算预留必须发生在上游调用前 | Reserve → Execute → Commit/Release |
| 3 | 路由结果必须进入证据链 | `channel_id` + `instance_id` |
| 4 | `usage` 来源必须显式标记 | `upstream` / `final_chunk` / `estimated` / `cached` |
| 5 | 流式错误不能伪装成正常成功 | 流中断/上游报错落 `failed`/`partial` 日志 + evidence，错误不伪装成功 |

## 开发

```bash
make test          # 全部测试（含覆盖率）
make test-repo     # 仓库层测试（每包独立 schema，可并行）
make test-race     # 竞态检测
make test-coverage # 覆盖率报告（coverage.html）
make lint          # go vet + staticcheck
make build         # 构建 bin/api + bin/worker
make run-api       # 启动 API 服务器
make run-worker    # 启动 Worker
make dev           # docker-compose up + API 启动
make web-dev       # 启动前端 Vite 开发服务器
make web-build     # 前端生产构建
```

开发遵循 ECC Harness 流水线：**planner → tdd-guide → code-reviewer → security-reviewer**，测试覆盖率 ≥ 80%。

## 前端技术细节

- **UI 组件**：shadcn/ui（Radix 原语）+ Tailwind CSS
- **状态管理**：TanStack Query（服务端状态）+ React Context（认证状态）
- **路由**：React Router v6
- **图表**：Recharts
- **Toast**：Sonner
- **统一状态组件**：LoadingState / ErrorState / EmptyState
- **错误边界**：ErrorBoundary + RouteErrorBoundary
- **测试**：Vitest + Testing Library + jsdom

## 生产部署

从本地到生产的切换只需修改 `.env`，详见 `.env.example` 中的注解：

| 变量 | 本地 | 生产 |
|------|------|------|
| `DATABASE_URL` | localhost | RDS / 云数据库 |
| `REDIS_URL` | localhost | Redis 服务（启用密码） |
| `CORS_ORIGIN` | localhost:5173 | 前端域名 |
| `COOKIE_SECURE` | **false** | **true**（HTTPS only） |
| `JWT_SECRET` | 任意 | `openssl rand -hex 32` |
| `ENCRYPTION_KEY` | 任意 32 字节 | `openssl rand -hex 16` |

前端构建：`cd web && npm run build` → 静态文件部署到 CDN / Nginx。

## Docker Compose 服务

| 服务 | 端口 | 说明 |
|------|------|------|
| `api` | 8080 | Go API 服务器 |
| `worker` | - | 后台任务（计费提交 / 健康检查 / 对账） |
| `web` | 3000 | React 前端（Vite HMR） |
| `postgres` | 5432 | PostgreSQL 16（持久化卷） |
| `redis` | 6379 | Redis 7 |

`docker compose up -d` 一键启动全部 5 个服务（含 worker）。

> **开发模式**：`docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build`
> 使用弱密钥 + 开启演示充值（`ENABLE_FAKE_PAYMENT=true`）。**dev 覆盖仅限本地，严禁用于生产**。

## 文档

| 文档 | 说明 |
|------|------|
| `docs/DEEPTROLS_完整功能清单.md` | 对照架构文档的全量功能矩阵（含最新变更） |
| `docs/PROJECT_STATUS.md` | 项目进度与变更记录 |
| `docs/AI聚合网关_完整文档.md` | 完整架构文档（10 篇 + 附录） |
| `docs/AI聚合平台_产品需求文档_PRD.md` | 产品需求文档 |

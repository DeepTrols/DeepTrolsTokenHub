# DeepTrols — AI Token 聚合平台

## 项目概述

企业级 AI Token（模型推理）聚合平台。不是反向代理，而是围绕模型调用构建的计费、风控、对账与运营系统。

## 架构

```
控制面 (Control)  → API Key / HMAC / 租户隔离 / 模型目录 / 限额
执行面 (Execution) → LiteLLM / Provider Adapter / 路由 / Fallback
资金面 (Money)    → Usage Log / Charge Line / 钱包 / 配额 / 价格快照
证据面 (Evidence) → Raw Usage / Provider Cost / Invoice / Release Evidence
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.22 + chi + pgx + Redis |
| 数据库 | PostgreSQL 16 |
| 缓存 | Redis 7 |
| 执行代理 | LiteLLM |
| 前端 | React 18 + TypeScript + Vite + Tailwind CSS |

## 开发工作流（强制）

**所有代码变更必须走 Agent Harness 流水线，禁止直接手写代码：**

```
1. planner agent      → 制定实现计划
2. tdd-guide agent    → 先写测试（RED），再实现（GREEN），重构
3. code-reviewer agent → 审查所有代码
4. security-reviewer   → 鉴权/计费/用户输入相关代码必须审查
```

**质量门禁：**
- 测试覆盖率 ≥ 80%
- 金额计算必须用 decimal，禁止 float
- 预算预留必须在上游调用之前
- 错误不能伪装成成功
- usage 来源必须显式标记

## 项目结构

```
cmd/api/main.go          # API 进程入口
cmd/worker/main.go       # Worker 进程入口
internal/
  domain/                # 领域模型
  handler/gateway/       # OpenAI-compatible 网关
  handler/console/       # 控制台 API
  handler/middleware/    # 鉴权/租户/限流中间件
  service/               # 业务逻辑（auth/billing/gateway/model/tenant）
  repository/            # 数据访问接口
  worker/                # 后台任务（健康检查/计费提交/对账）
  pkg/                   # 工具包（decimal/幂等/usage解析）
migrations/              # PostgreSQL DDL
web/                     # React 前端
```

## 快速启动

```bash
# 基础设施
docker-compose up -d

# 数据库迁移
migrate -path migrations -database "$DATABASE_URL" up

# 后端
export $(cat .env | grep -v '^#' | xargs)
go run ./cmd/api

# 前端
cd web && npm install && npm run dev
```

## 5 个不变量

1. `request_id` 不是全局唯一账务身份（需 `tenant+user+key+type+request_id`）
2. 预算预留必须发生在上游调用前
3. 路由结果必须进入证据链
4. `usage` 来源必须显式标记（upstream / final_chunk / estimated）
5. 流式错误不能伪装成正常成功

## 参考文档

- `docs/AI聚合平台_产品需求文档_PRD.md` — 产品需求文档
- `docs/AI聚合网关_完整文档.md` — 完整架构文档
- `.claude/plans/mvp-core-platform.plan.md` — MVP 实施计划
- `docs/PROJECT_STATUS.md` — 项目进度与变更记录

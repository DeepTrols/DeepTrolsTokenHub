# DeepTrols — AI Token 聚合平台

## 项目概述

企业级 AI Token（模型推理）聚合平台。不是反向代理，而是围绕模型调用构建的计费、风控、对账与运营系统。

## 架构

```
控制面 (Control)  → API Key / HMAC / 租户隔离 / 模型目录 / 限额
执行面 (Execution) → OpenAI 兼容直连 / Provider Adapter / 路由 / Fallback
资金面 (Money)    → Usage Log / Charge Line / 钱包 / 配额 / 价格快照
证据面 (Evidence) → Raw Usage / Provider Cost / Invoice / Release Evidence
```

## 技术栈

| 层 | 技术 |
|---|---|
| 后端 | Go 1.22 + chi + pgx + Redis |
| 数据库 | PostgreSQL 16 |
| 缓存 | Redis 7 |
| 执行代理 | OpenAI 兼容直连（渠道实例 base_url；内置 LiteLLM 已于 2026-08-19 移除） |
| 前端 | React 18 + TypeScript + Vite + Tailwind CSS |

## 开发工作流

按变更风险分级使用 TDD，不强制全员流水线；质量统一收口到"对现实验证"。

**核心路径（计费 / 鉴权 / 网关 / 证据链）— TDD 默认：**
先写失败测试（RED）把不变量固化成断言，再实现（GREEN）、重构。这类代码 bug 成本高，
测试是最可靠的安全网；不写测试直接改属于例外，需说明理由。

**非核心（脚本 / 文档 / 配置 / 迁移 / 机械重构 / 测试基建）— 轻量：**
不强制 TDD；改完必须跑一次全量验证（并行 `go test ./...`、`go vet`、`go build`、
gofmt），并尽量补一条针对真实问题的回归测试。

**所有变更完成前，必须对现实验证一次**（至少满足其一）：
- 全量并行测试全绿；
- 针对真实环境的调用 / 探针验证（真实进程、真实数据库、检查产出物内容）；
- 明确说明无法验证的原因。

**审查（简化）：**
- code-reviewer：核心路径变更必须审，非核心可选；审查者只读，禁止提交。
- security-reviewer：仅鉴权 / 计费 / 用户输入相关代码必须审。
- 流程服务于质量：不得为"走完流程"嵌套多层 agent 或机械套用角色。

**质量门禁：**
- 金额计算必须用 decimal，禁止 float
- 预算预留必须在上游调用之前
- 错误不能伪装成成功
- usage 来源必须显式标记
- 测试覆盖率 ≥ 80%（愿景目标，CI 尚未强制）

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
- `docs/DEEPTROLS_完整功能清单.md` — 架构对照实现清单
- `docs/PROJECT_STATUS.md` — 项目进度与变更记录

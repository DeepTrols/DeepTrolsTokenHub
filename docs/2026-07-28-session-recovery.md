# 会话恢复指南

> 创建: 2026-07-28

## 恢复方法

```
继续上次的工作，参考 docs/2026-07-28-session-recovery.md
```

## 当前流水线

```
✅ Phase 1-4: Repository 实现 + 服务层 + Handler 集成
📋 待办: 代码审查 → 模型管理 CRUD → Console Handler DB 集成
```

## 恢复后第一步

1. `docker compose ps` — 确认基础设施运行中
2. `export $(cat .env | grep -v '^#' | xargs) && go test -p 1 ./internal/repository/... -count=1` — 验证测试
3. `go build ./... && ./bin/api.exe` — 启动后端
4. `cd web && npx vite --port 3000` — 启动前端

## 待办功能

### 模型管理 CRUD（详见 PROJECT_STATUS.md）

底层已就绪（models 表 + model.Repository），缺控制台管理界面：

- 后端: `console/models.go` — Create/Update/Delete/Get 4 个 handler
- 路由: `main.go` — POST/PUT/DELETE/GET `/api/console/models`
- 前端: `web/src/pages/Models.tsx` — 表格 + 创建/编辑 Modal
- 前端 API: `web/src/api/models.ts` — 4 个 API client 函数

## 测试命令

```bash
go test -p 1 ./internal/repository/... -cover -count=1   # 必须 -p 1（共享数据库）
make test-repo   # 同上
```

## 覆盖率 (2026-07-28)

| 包 | 覆盖率 |
|---|---|
| pkg/db | 89.3% |
| apikey | 89.5% |
| channel | 87.3% |
| model | 82.0% |
| tenant | 85.7% |
| usage | 92.4% |
| wallet | 83.9% |

## 新增文件 (本次会话)

- `internal/repository/usage/postgres.go` + `postgres_test.go`
- `internal/repository/tenant/postgres.go` + `json.go` + `postgres_test.go`
- `internal/repository/channel/postgres.go` + `postgres_test.go`
- `internal/service/billing/charger.go`
- `internal/service/billing/logger.go`

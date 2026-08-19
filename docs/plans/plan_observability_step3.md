# 可观测性 Step 3 实施计划

## 目标

1. 结构化日志 + 请求日志中间件（stdlib `log/slog` JSON，零新依赖；Sentry/OTel 错误上报列为后续可选项）。
2. `/healthz`（存活）+ `/readyz`（依赖就绪：DB/Redis 探活）双端点，`/health` 保持向后兼容。
3. Worker 分布式选主：**已实现**（`internal/pkg/lease` + `cmd/worker/main.go` withLease，含 lease_test.go），本计划不重复实施，仅验证。

## 现状（已核实）

- `internal/app/app.go`：`App` 持有 `Pool *pgxpool.Pool`、`Redis *goredis.Client`、`Config`。
- `internal/app/health.go`：`/health` 恒返回 `{"status":"ok"}`，不探依赖。
- `cmd/api/main.go`：chi 路由，`RegisterRoutes` 注册 `/health`；无请求日志中间件；全仓用 `log.Printf`（stdlib 非结构化）。
- `internal/handler/middleware/`：已有 SecurityHeaders/CORS/GatewayAuth/ConsoleAuth/限流/审计中间件。
- `go.mod`：无 slog 之外的依赖（`log/slog` 为 stdlib，Go 1.22 内置）。

## 改动

### A. 结构化日志（零新依赖）

1. 新增 `internal/app/logger.go`：
   - `func NewSlogLogger(cfg *config.Config) *slog.Logger`：按 `LOG_FORMAT`（json|text，默认 json）与 `LOG_LEVEL`（debug|info|warn|error，默认 info）创建 `slog.JSONHandler`/`slog.TextHandler`。
   - `App.Logger *slog.Logger` 字段；`NewApp` 初始化（nil 时退回默认 info JSON）。
2. `internal/config/config.go`：`ServerConfig`/`Config` 增加 `LogFormat string`、`LogLevel string`（`envOrDefault`），`validate` 校验格式枚举（非法回退默认，不 fail-fast 以免影响启动）。
3. `internal/handler/middleware/requestlog.go`：新增 `RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler`：
   - 记录 method、path、status、duration_ms、request_id（X-Request-ID，无则生成）、client_ip、user_id（CtxUserID，若有）、api_key_id（CtxAPIKeyID，若有）；
   - 包装 `ResponseWriter` 捕获状态码；
   - `logger.Info("http_request", ...)`，level 按状态码（5xx→Error，4xx→Warn，其余 Info）；
   - 不记录请求/响应体（避免敏感数据）。
4. `cmd/api/main.go`：`r.Use(middleware.RequestLogger(application.Logger))`（CORS 之后、业务路由之前，安全头保持最先）。
5. 遗留 `log.Printf` 调用不迁移（记录为后续项）；关键网关失败日志已在 evidence 落库。

### B. 健康检查双端点

1. `internal/app/health.go` 重构：
   - `healthHandler`（`/health`）保持原样（向后兼容）；
   - 新增 `healthzHandler`（`/healthz`）：进程存活恒 200 `{"status":"ok"}`；
   - 新增 `readyzHandler(application *app.App)`（`/readyz`）：
     - 2s timeout context；
     - `Pool.Ping` → 失败则 503；
     - Redis 配置且非 nil → `Redis.Ping`；失败则 503（Redis 不可用时 API 已有内存降级，但就绪语义按依赖判定，注释说明）；
     - 响应 `{"status":"ready","checks":{"database":"ok","redis":"ok"}}`，任一失败 `{"status":"not_ready","checks":{...:"error"}}` + 503；
     - 通过 `RegisterRoutes` 注册 `/healthz`、`/readyz`（readyz 需要 App 引用，`RegisterRoutes(a *App)` 已有 receiver）。
2. 兼容性：`/health` 行为不变，现有探活/文档不受影响。

### C. 验证与测试（TDD，RED→GREEN）

- `middleware/requestlog_test.go`：捕获输出断言字段（method/path/status/duration/request_id）；5xx 记 Error、4xx 记 Warn、2xx 记 Info；无 X-Request-ID 时生成；不含请求体。
- `app/health_test.go` / `app_router_test.go`：`/healthz` 恒 200；`/readyz` 在 DB 可达时 200 且 checks.database=ok（集成，`testutil.SetupPool`）；`/health` 回归不变。
- `config/config_test.go`：LOG_FORMAT/LOG_LEVEL 解析与非法值回退。
- 全量：`go vet ./...`、`go build ./...`、`go test -p 1 ./...`（GOCACHE 指向临时目录；repository 测试依赖真实 PG，串行）。

## 边界 / 风险

- 请求日志中间件放在 CORS 之后，OPTIONS 预检也记录（可接受）；安全头中间件保持最先。
- 不记录 body/query 敏感参数；仅记录路径（不含 query）。
- `/readyz` 在 Redis 未配置（开发单机）时跳过 Redis 检查不报 503；配置了但不可达则 503（就绪语义明确）。
- 不引入新依赖；错误上报（Sentry/OTel）留作后续项并记录。

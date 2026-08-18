# 实施计划：非流式证据补齐 → 管理后台限流+生产基线 → 网关端点扩展

## 总览

按用户确认的优先级顺序实施三个事项，每项独立交付、独立验证：

1. **非流式上游失败证据补齐**（1 天）：`/v1/chat/completions` 非流式请求在上游失败时不再"消失"，落 usage_log（failed，零成本，带证据）。
2. **管理后台限流 + 生产配置基线**（1 天）：`/api/admin` 加限流中间件；生产模式强制 `COOKIE_SECURE=true` 与管理员密码长度 ≥12；补 `.env.example` 生产注释与 `docs/DEPLOYMENT.md`。
3. **网关端点扩展**（3–5 天）：新增 `/v1/embeddings`、`/v1/images/generations`、`/v1/audio/speech`，走完整计费链路（预留→执行→结算→usage_log），失败同样落证据。

约束（AGENTS.md）：金额一律 decimal；预算预留在上游调用前；usage 来源显式标记；错误不伪装成功；Go 测试必须 `go test -p 1 ./...` 串行。

---

## 事项 1：非流式上游失败证据补齐

### 现状（已核实）

- `internal/handler/gateway/chat.go` `HandleNonStreamingChat`：候选循环内每次失败已 release 预留金并置 `reserveResult=nil`；循环结束后 `upstreamFailed=true` 时只 release 配额、`writeError` 后 return，**不落 usage_log**。
- 循环内保留 `lastErr` 与最后一个 `resp`（失败时 resp 可能非 nil 且 StatusCode≥400，body 已由 executor 解析，≤10MB）。
- 流式路径已有 `logStreamFailure`（detached context 30s、状态 failed/partial、零成本、evidence 带 status_code/body/error）——非流式复用同一模式。
- 风险点：失败收尾的 release 使用 `r.Context()`；若客户端已断开（context canceled），release 会失败 → 预留金冻结。需改为 detached context。
- `resolveStreamAPIKeyID` 已存在；非流式用 `resolveIdentity` 得到 userID/apiKeyID。

### 改动

**`internal/handler/gateway/chat.go`**

1. 循环内：失败时记录最后一次尝试的 `resp.StatusCode`、`resp.DurationMs`、错误体（`resp.Body`），供最终失败日志使用（新增局部变量 `lastStatus int`、`lastResp *gw.ExecuteResponse`、`attemptCount`；`lastErr` 已存在）。
2. 循环内 release：新增 helper `releaseHoldSafe(ctx, application, reserveResult, quotaReservation, requestID)`——若 `ctx.Err() != nil`（客户端断开）则改用 `context.WithTimeout(context.Background(), 30s)` 再 release；成功路径保持现状。循环内与最终失败路径都调用它。
3. 新增 `logNonStreamFailure(application, userID, apiKeyID, tenantID, modelName, upstreamModel, routeResult, body, requestID, lastErr, lastResp, attemptCount, durationMs)`：
   - detached context（30s）
   - `Status=failed`；`ErrorCode`：`lastErr != nil` → `upstream_error`，`lastResp.StatusCode>=400` → `upstream_http_error`
   - `UsageSource=estimated`；成本/钱包扣费/配额恒 0
   - evidence：`Provider="litellm"`、`RequestBody=body`、`ResponseBody=lastResp.Body`（marshal 后超 `maxUpstreamErrorBody` 1MB 截断）、`StatusCode=lastStatus`、`DurationMs`、`ErrorMessage`
   - `RequestType="chat"`；channel/instance/routePolicyID 从 routeResult（最后一个候选）取
4. `upstreamFailed` 分支：release（detached）后、`writeError` 前调用 `go logNonStreamFailure(...)`。

### 测试清单（先 RED）

在 `internal/handler/gateway/chat_test.go` 新增（复用现有 mockExecutor / mockWalletRepo / mockUsageRepo / mockLogger 装配）：

1. `TestHandleNonStreamingChat_UpstreamHTTPError_LogsFailed`：mock 返回 StatusCode=500 + JSON body → 响应 502/透传、release 被调、usage_log 记录 failed、ErrorCode=upstream_http_error、evidence.StatusCode=500、成本 0。
2. `TestHandleNonStreamingChat_UpstreamConnectionError_LogsFailed`：mock 返回 error → failed、ErrorCode=upstream_error、release 被调。
3. `TestHandleNonStreamingChat_FailoverAllFailed_LogsSingleFailure`：多候选全失败 → 仅一条失败日志、evidence 为最后一次尝试、attempt 信息在错误消息中可追溯。
4. `TestHandleNonStreamingChat_ClientCancel_ReleasesHold`：带已取消的 context 发起 → release 仍被调用（detached 兜底），日志仍落（detached）。
5. `TestHandleNonStreamingChat_Success_NoFailureLog`：成功路径不产生 failed 日志（回归）。

### 边界情况

- 上游错误体超大（>1MB）：evidence 截断，不 OOM、不撑大表。
- 客户端断开：release 与日志均走 detached context。
- 所有候选失败：只落一条日志（最终尝试证据），避免重复记录。
- 无 resp（连接错误）：evidence.StatusCode=0、ResponseBody 空、ErrorMessage=lastErr。

---

## 事项 2：管理后台限流 + 生产配置基线

### 现状（已核实）

- `internal/handler/middleware/ratelimit.go`：已有 LoginRateLimit/GatewayRateLimit/TeamRateLimit，无 Admin。
- `cmd/api/main.go` `/api/admin` 组：ConsoleAuth → AdminAuth → AuditAdminWrite，无限流。
- `internal/config/config.go`：`validate()` 在 `!FakePayment` 时拒绝 4 个弱默认值；`COOKIE_SECURE` 默认 true 但未强制；管理员密码无长度校验。
- `.env.example` 无生产环境说明；无部署文档。

### 改动

**`internal/handler/middleware/ratelimit.go`**

新增 `AdminRateLimit(limiter, limit, window)`：与 TeamRateLimit 同构，key 前缀 `rl:admin:`，优先用户 ID（CtxUserID）、IP 兜底；注释说明阈值 120/min 是按"读多写少的后台操作"取的保守值，后续可按路由细分。

**`cmd/api/main.go`**

`/api/admin` 组在 `AdminAuth()` 之后、`AuditAdminWrite` 之前加 `r.Use(middleware.AdminRateLimit(application.RateLimiter, 120, 1*time.Minute))`。

**`internal/config/config.go` `validate()`**

`!FakePayment`（生产模式）追加两条：
- `!c.Cookie.Secure` → error（"COOKIE_SECURE must be true in production"）
- `len(c.Bootstrap.AdminPassword) < 12` → error（弱密码拒绝启动）

**`.env.example`**

新增"生产环境（Production）"注释块：`ENABLE_FAKE_PAYMENT=false`、`COOKIE_SECURE=true`、`COOKIE_SAMESITE=Strict`、强随机密钥生成命令（openssl rand -hex 32 / 16）、TLS 终止说明（前置 nginx/Caddy/云 LB，本服务仅 HTTP 回环）。

**`docs/DEPLOYMENT.md`（新增）**

部署清单：前置 TLS 终止；环境变量基线（上表）；迁移执行与回滚步骤（含 dirty 修复：`migrate force <version>` 前提与后果）；健康检查（/health 与后续 /readyz）；数据库备份；密钥轮换说明；灰度/回滚流程。

### 测试清单（先 RED）

- `internal/handler/middleware/ratelimit_test.go`：`TestAdminRateLimit_BlocksAfterLimit`（同用户超限 429 + Retry-After；不同用户互不影响；无用户时按 IP）。
- `internal/config/config_test.go`：`TestValidate_ProductionRequiresSecureCookie`（FakePayment=false + COOKIE_SECURE=false → error）；`TestValidate_ProductionRequiresStrongAdminPassword`（长度<12 → error）；`TestValidate_DevModeAllowsDefaults`（FakePayment=true 全默认 → ok，回归）。

---

## 事项 3：网关端点扩展（embeddings / images / audio）

### 现状（已核实）

- `internal/service/gateway/executor.go`：`Executor.Execute` 拼死 `/v1/chat/completions`；无通用端点方法。
- `internal/handler/gateway/chat_test.go` `mockExecutor` 仅实现 `Execute`（唯一 executor mock）。
- `internal/pkg/usageparser/parser.go`：`NormalizedUsage` 已含 ImageCount/AudioSeconds/TTSCharacters；`ParseOpenAIUsage` 已能解析 `prompt_tokens`（embeddings 可直接复用，source=upstream）。
- `internal/service/billing/pricer.go`：dimensions map 已含 input/output/image/audio/tts，按维度计价；`ModelPricing.RequestType` 列存在但 pricer 未按 request_type 过滤——**本事项不改 pricer 的匹配语义**（避免破坏 chat 计费），仅靠模型维度定价；如需 request_type 隔离另立项。
- `internal/service/gateway/router.go`：按 model code 路由、channel 绑定 model ID，无需按 category 过滤。
- `cmd/api/main.go`：/v1 只有 models + chat/completions。

### 改动

**`internal/service/gateway/executor.go`**

接口增加 `ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*ExecuteResponse, error)`；`Execute` 改为调用 `ExecuteEndpoint(..., "chat/completions", ...)`，行为不变。

**`internal/handler/gateway/chat_test.go`**

`mockExecutor` 增加 `ExecuteEndpoint` 实现（默认委托给 executeFn，另加可选 executeEndpointFn 字段用于端点专项测试）；补一个编译期断言 `_ gw.Executor = (*mockExecutor)(nil)`（已有）。

**新增 `internal/handler/gateway/endpoints.go`**

通用助手 `handleForwardedEndpoint(w, r, application, endpoint, requestType, estimateUsage)`：
- 方法校验 POST、1MB body、model 必填、`sanitizeRequestBody`
- `enforceAPIKeyBoundaries`（复用，需核实其是否按 request_type 区分——现状按 model 校验，足够）
- 路由候选（3 个 failover）、预留金（最小 0.0001 兜底）、配额预留（按估算 token）
- `Executor.ExecuteEndpoint` 逐候选执行，失败 release + 换下一个
- 成功后：从响应解析/估算 usage → pricer 计价 → Settle → 配额 Settle → `recordAPIKeySpend` → `go logUsage(...)`（request_type 参数化，新增 `logEndpointUsage` 复用 logUsageWithCosts 结构）
- 全失败：`logNonStreamFailure`（事项 1 的 helper 参数化 request_type 后复用）

三个端点：

1. `HandleEmbeddings`：endpoint=`embeddings`；usage=响应 `usage.prompt_tokens`（`ParseOpenAIUsage`），无 usage 时按输入文本长度估算；request_type=`embeddings`。
2. `HandleImagesGenerations`：endpoint=`images/generations`；usage=请求 `n`（默认 1）→ `ImageCount`；无响应 usage 时用 `NormalizedUsage{ImageCount:n}`；request_type=`images`。
3. `HandleAudioSpeech`：endpoint=`audio/speech`；usage=估算输入文本字符/4 → `TTSCharacters`；request_type=`audio`。（audio/transcriptions 为 multipart，本事项不做，计划中标注后续项。）

**`cmd/api/main.go`**

/v1 组内追加：
```go
r.Post("/embeddings", gateway.HandleEmbeddings(application))
r.Post("/images/generations", gateway.HandleImagesGenerations(application))
r.Post("/audio/speech", gateway.HandleAudioSpeech(application))
```

**`internal/domain/usage.go` / `internal/pkg/usageparser`**

如需新 request_type 常量则加（如 `RequestTypeEmbeddings/Images/Audio`），或沿用字符串字面量并在 handler 中定义常量；不新增迁移。

### 测试清单（先 RED）

新增 `internal/handler/gateway/endpoints_test.go`（复用 chat_test 的装配函数）：

1. embeddings 成功：mock 响应含 `usage.prompt_tokens` → Settle 按 input 计价、usage_log.RequestType=embeddings、UsageSource=upstream、charge line dimension=input。
2. embeddings 无 usage：按估算落库，source=estimated。
3. images 成功：`{"n":2}` → ImageCount=2、dimension=image、log 成功。
4. audio/speech 成功：输入文本 → TTSCharacters>0、dimension=tts。
5. 每个端点上游 4xx：透传、release、failed 日志带 evidence（参数化子测试覆盖三种端点）。
6. 每个端点无 wallet / 余额不足：402，预留金不产生结算。
7. 每个端点未知模型：404 model_not_found（回归路由错误映射）。

### 边界情况

- embeddings 响应 usage 字段缺失/非标准 → estimated 兜底，不 panic。
- images `n` 缺失/非法 → 默认 1；超大 n（如 >10）→ 400（防刷成本）。
- audio/speech 无输入文本 → 400。
- 所有端点的失败路径零成本 + 证据，与 chat 一致。

---

## 验证（每项完成后）

- `go vet ./...`、`go build ./...`、`go test -p 1 ./...`（全量）
- 迁移：`migrate -path migrations -database ... up` + down 往返（仅事项 1/3 涉及库改动时；本计划预计无新迁移）
- 全量通过后重启 API 进程并验证 /health
- 更新 `docs/PROJECT_STATUS.md`

## 风险与规避

- **Executor 接口变更**：编译期强制所有实现（含测试 mock）同步；先改接口+mock 再改调用方。
- **release 语义**：只把失败路径改为 detached 兜底，成功路径不动，避免影响正常结算。
- **pricer 语义不动**：新端点只填 pricer 已支持的维度，不引入 request_type 过滤，防止 chat 计价回归。
- **限流误伤**：admin 阈值 120/min 远高于人工操作，且 key 按用户；失败时 fail-open（与现有中间件一致）。
- **生产校验误拒**：只追加两条硬校验，默认值（COOKIE_SECURE=true）即可通过；开发模式（FakePayment=true）不触发。

## 成功标准

- [ ] 非流式失败请求全部有 usage_log（failed，零成本，带证据），对账不再有"消失的请求"
- [ ] /api/admin 超限返回 429；生产模式错误配置拒绝启动
- [ ] embeddings/images/audio 三个端点走通"预留→执行→结算→日志"，失败落证据
- [ ] `go test -p 1 ./...` 全绿；PROJECT_STATUS.md 更新

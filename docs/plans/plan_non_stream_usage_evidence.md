# 非流式调用证据补齐实现计划（Item 1）

## 目标

非流式 `POST /v1/chat/completions`（stream=false）在上游全部失败时，当前只释放预留金并返回 502/错误，**不写 usage_log / provider_evidence**，这些调用在对账中是"账外请求"。本计划补齐：失败请求落 `usage_log`（status=failed），证据带上游状态码与错误体，成本恒 0，不破坏成功路径。

## 现状（已核实）

- `internal/handler/gateway/chat.go` `HandleNonStreamingChat`：
  - 每个候选失败时在循环内 `Release` 该次 hold 并置 `reserveResult = nil`；
  - 全部失败进入 `if upstreamFailed { ... writeError(502, "upstream_error") ; return }`——**不落日志**；
  - 成功路径走 `logUsageWithCosts`（completed）。
- 流式路径已有参照：`logStreamFailure`（detached context 30s、failed/partial、错误码分类、1MB 错误体截断、成本 0）与 `releaseIfReserved`。
- `internal/domain/usage.go`：`UsageLogStatusFailed`、`UsageSourceEstimated`、evidence 字段已存在。
- 非流式 `ExecuteResponse.Body` 是 `map[string]any`；流式错误体读取参照 line ~446 `io.LimitReader(resp.Body, maxUpstreamErrorBody)`——需确认 `gw.ExecuteResponse` 是否带原始错误体（`RawBody`/`ErrorBody`），若没有则退化为只记 statusCode + lastErr。

## 改动

### 1. `internal/handler/gateway/chat.go`

a) 循环内新增记录最后一次失败的 `lastResp *gw.ExecuteResponse`（每次失败覆盖）。

b) `if upstreamFailed` 分支改为：

```go
if upstreamFailed {
    // 1) release 预留金与 quota（保持现状，循环内已逐个 release hold）
    if application.QuotaChecker != nil {
        application.QuotaChecker.Release(r.Context(), quotaReservation, requestID)
    }
    // 2) 落失败证据（detached，不依赖客户端连接）
    go logNonStreamingFailure(application, userID, apiKeyID, tenantID, modelName,
        upstreamModelOfLastAttempt, body, requestID, lastErr, lastResp, attempts)
    // 3) 透传错误（保持现状）
    msg := "Upstream request failed"
    if lastErr != nil { msg = lastErr.Error() }
    writeError(w, http.StatusBadGateway, "upstream_error", msg)
    return
}
```

c) 新增 `logNonStreamingFailure(...)`（复制 `logStreamFailure` 的结构，参数：userID/apiKeyID/tenantID/modelName/upstreamModel/body/requestID/lastErr/lastResp/attempts）：
- detached context 30s；
- `Status = UsageLogStatusFailed`（非流式失败前未写出任何内容）；
- `UsageSource = UsageSourceEstimated`（无最终 usage）；
- 成本 0、WalletCharged 0、charge lines 空；
- `ErrorCode` 分类：
  - `errors.Is(lastErr, context.Canceled)` → `client_disconnected`
  - `lastResp != nil && lastResp.StatusCode >= 400` → `upstream_http_error`（evidence.StatusCode = lastResp.StatusCode，错误体截断 1MB）
  - 其它 → `upstream_error`
- evidence：`Provider="litellm"`、status_code、response_body（截断）、error_message（lastErr.Error() 或 HTTP 错误体摘要）、request_body（沿用 Phase 1 约定）、tenant_id；
- `durationMs` 从请求开始计时（调用方传入 `int(time.Since(start).Milliseconds())`，如无非流式计时点则记录 0，不扩 scope）；
- `attempts` 数量写入 error_message 或 evidence 摘要（如 `"all N candidates failed"`）。

d) 若 `gw.ExecuteResponse` 无原始错误体字段：错误体记录改为 lastErr 文本 + statusCode；不新增 executor 接口改动（避免扩 scope），在计划中注明。

### 2. 测试（`internal/handler/gateway/chat_test.go`，RED 先行）

- 更新既有非流式失败断言（若原断言"不写日志"，改为"写 failed 日志"）。
- 新增：
  1. `TestHandleNonStreamingChat_UpstreamHTTPError_LogsFailed`：executor 返回 500 → 客户端 502、releaseCalled==1、log.Status==failed、ErrorCode==upstream_http_error、evidence.StatusCode==500、cost==0；
  2. `TestHandleNonStreamingChat_UpstreamConnectionError_LogsFailed`：executor 返回连接错误 → failed、upstream_error、release 调用；
  3. `TestHandleNonStreamingChat_AllCandidatesExhausted`：2 候选全失败 → 单条 failed 日志、每个候选 hold 均 release、message 含 attempts；
  4. `TestHandleNonStreamingChat_ClientCancel_LogsFailed`：context canceled → failed、client_disconnected；
  5. `TestHandleNonStreamingChat_ErrorBodyTruncated`：错误体 >1MB → 落库 ≤1MB；
  6. 成功路径回归：既有 `...Success...` 用例仍 completed、响应结构不变。

### 3. 验证

- `gofmt -l internal/handler/gateway/`（注意 CRLF 误报，仅看本次改动文件）
- `go vet ./...`、`go build ./...`
- `go test -p 1 ./internal/handler/gateway/ ./internal/service/billing/` 及最终全量 `go test -p 1 ./...`（GOCACHE 指向临时目录）

## 兼容性 / 风险

- 不改成功路径、不改响应结构、不改前端；
- `failed` 状态与对账 `findErrorMislabel` 语义一致（不再出现"evidence≥400 但 completed"）；
- 失败日志全部 detached，客户端断连不丢证据；
- 若 executor 错误体字段不可得，只降级记录，不影响功能。

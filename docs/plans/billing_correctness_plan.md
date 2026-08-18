# 计费正确性收尾实现计划（Step 1）

## 目标

修复三个 Blockers，全部属于资金面/证据面核心风险：

1. **流式 usage 来源标记缺失**：`final_chunk` 常量已定义但从未使用（不变量 #4）
2. **流式错误伪装成功**：`usage_log` 状态恒为 `completed`，流中断/上游报错路径不落任何日志（不变量 #5）
3. **价格快照无证据**：`price_snapshot` 始终为空 map，无定价版本与来源

## 关键现状（探索结论）

- `internal/domain/usage.go`：`UsageSourceFinalChunk` 已定义；`UsageLogStatus` 已有 `failed/partial/completed/refunded`
- `internal/pkg/usageparser/parser.go`：`SourceFinalChunk` 已定义，从未使用
- `internal/handler/gateway/chat.go`：
  - `HandleStreamingChat` 从最后 chunk 解析 usage 时写死 `usageSource = usageparser.SourceUpstream`（应为 `SourceFinalChunk`）
  - `logStreamUsage` 硬编码 `Status: domain.UsageLogStatusCompleted`
  - 失败路径（client.Do 错误、upstream >= 400、flusher 不支持、scanner.Err()）只做 `releaseIfReserved`，**不落 usage_log**（证据缺失）
  - scanner 错误时已不发送 `[DONE]`（部分修复过），但无失败日志
- `internal/service/billing/pricer.go`：`PriceResult.PriceSnapshot` 初始化为空 map；`ChargeLineInput` 已有 `PriceSource` 字段（从未赋值），仅缺 `PriceVersion` 字段
- `internal/service/billing/logger.go`：`insertChargeLines` 使用 params 级 `ChargeLineSource/ChargeLineVer`，per-line 未接入
- `internal/repository/model/postgres.go`：`FindByModel` 未 SELECT `price_version`（表无此列）
- `migrations/000001_init.up.sql`：`usage_logs.usage_source` CHECK 为 `('upstream','final_chunk','estimated')`，**不含 `cached`**——而 `logUsageCacheHit` 写 `UsageSourceCached`，落库必被 CHECK 拒绝（缓存命中零证据的潜在 bug）
- 对账 worker `findErrorMislabel` 会把"evidence.status_code >= 400 且 usage_log.status = completed"标为 critical——失败路径若写成 completed 会被对账抓到（行为正确，不能破坏）
- 前端不消费 `usage_source` 字段（仅 cost 字段），改值无前端影响
- 现有测试 `TestHandleStreamingChat_SuccessWithUsage` 断言 `UsageSource == UsageSourceUpstream`——行为变更后必须同步更新为 `FinalChunk`（预期 RED）

## 改动明细

### A. 数据库迁移（新增 000009）

`migrations/000009_billing_evidence.up.sql`：

```sql
ALTER TABLE model_pricing ADD COLUMN price_version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE usage_logs DROP CONSTRAINT usage_logs_usage_source_check;
ALTER TABLE usage_logs ADD CONSTRAINT usage_logs_usage_source_check
  CHECK (usage_source IN ('upstream', 'final_chunk', 'estimated', 'cached'));
```

`migrations/000009_billing_evidence.down.sql`：

```sql
ALTER TABLE model_pricing DROP COLUMN price_version;

ALTER TABLE usage_logs DROP CONSTRAINT usage_logs_usage_source_check;
ALTER TABLE usage_logs ADD CONSTRAINT usage_logs_usage_source_check
  CHECK (usage_source IN ('upstream', 'final_chunk', 'estimated'));
```

风险：CHECK 收紧方向为先宽后窄，down 恢复原状；现库中不存在 `cached` 行（此前写入必失败），无脏数据。

### B. 定价版本写入路径

- `internal/handler/console/pricing.go` `HandleSetMarkup`：UPDATE 增加 `price_version = price_version + 1`
- `internal/domain/model.go`：`ModelPricing` 增加 `PriceVersion int64`
- `internal/repository/model/postgres.go` `FindByModel`：SELECT 增加 `price_version`，scan 到 `p.PriceVersion`

### C. Pricer：快照 + 计费行元数据

`internal/service/billing/pricer.go`：

- `ChargeLineInput` 仅需增加 `PriceVersion int`（`PriceSource string` 已存在，补赋值即可）
- 每个实际计费维度：`PriceSource = "model_pricing"`、`PriceVersion = int(pr.PriceVersion)`
- `PriceResult.PriceSnapshot` 填充：
  ```json
  {
    "source": "model_pricing",
    "currency": "CNY",
    "captured_at": "<RFC3339 UTC>",
    "rows": [
      {"pricing_id": "...", "dimension": "input", "unit_name": "token",
       "unit_price": "0.000015", "upstream_cost": "0.000010",
       "price_version": 1, "tenant_id": null}
    ]
  }
  ```
  只包含实际用于计费的行；无定价行时 `rows` 为空数组但 `source/captured_at` 仍在。decimal 一律字符串，禁止 float。

### D. Logger：per-line 来源/版本

`internal/service/billing/logger.go` `insertChargeLines`：优先使用 `lines[i].PriceSource/PriceVersion`，为空时回退 `params.ChargeLineSource/ChargeLineVer`（兼容现有调用与测试）。

### E. 网关：final_chunk 标记 + 失败/部分日志

`internal/handler/gateway/chat.go`：

1. 最后 chunk 解析到 usage 时：`usageSource = usageparser.SourceFinalChunk`（替换 `SourceUpstream`）
2. `domainUsageSource` 映射增加 `final_chunk → domain.UsageSourceFinalChunk` 分支
3. 扫描循环增加 `chunksForwarded` 计数（每转发一条 data 加一）
4. 新增 `logStreamFailure(application, userID, apiKeyID, modelName, upstreamModel, routeResult, body, lastDataLine, chunksForwarded, status, errorCode, errorMessage, upstreamStatusCode, upstreamBody, durationMs)`：
   - detached context（30s，不依赖 r.Context()）
   - `Status`：`partial`（chunksForwarded > 0）或 `failed`
   - `ErrorCode`：`upstream_error` / `upstream_http_error` / `streaming_not_supported` / `stream_interrupted` / `client_disconnected`
   - `UsageSource`：若失败前已捕获 usage 则 `final_chunk`，否则 `estimated`
   - 成本 0、`WalletCharged` 0（reserve 已 release）
   - evidence：`Provider="litellm"`、上游 status_code、response_body（错误体）、error_message
5. 失败路径接入：
   - `client.Do` 错误 → release + `logStreamFailure(failed, upstream_error)`
   - `resp.StatusCode >= 400` → 透传错误体 + release + `logStreamFailure(failed, upstream_http_error)`
   - flusher 不支持 → release + `logStreamFailure(failed, streaming_not_supported)`
   - `scanner.Err()` → 不发送 [DONE]（保持）+ release + `logStreamFailure(partial|failed, stream_interrupted 或 context.Canceled→client_disconnected)`
6. `logStreamUsage` 增加 `status domain.UsageLogStatus` 参数，干净 EOF 时显式传 `completed`，删除硬编码

### F. 非流式说明

非流式上游 HTTP 错误（executor 返回 StatusCode>=400）目前也不落日志——属于相邻证据缺口，**本计划不扩scope**，留作后续项，仅在本文件记录。

## 测试清单（tdd-guide 的 RED 顺序）

1. `chat_test.go` 更新 `TestHandleStreamingChat_SuccessWithUsage`：期望 `UsageSourceFinalChunk`，新增断言 `Status == completed`、evidence 已落
2. 新增 `TestHandleStreamingChat_TruncatedStream_LogsPartialAndReleases`：upstream 发若干 chunk 后异常断连（无 [DONE]）→ 响应无 [DONE]、`releaseCalled==1`、`settleCalled==0`、log.Status==partial、ErrorCode==stream_interrupted、WalletCharged==0
3. 新增 `TestHandleStreamingChat_UpstreamHTTPError_LogsFailed`：upstream 500 JSON → 状态透传、release、log.Status==failed、ErrorCode==upstream_http_error、evidence.StatusCode==500
4. 新增 `TestHandleStreamingChat_UpstreamConnectionError_LogsFailed`：连接失败 → failed、ErrorCode==upstream_error、release
5. `pricer_test.go`：
   - `TestPricer_PriceSnapshot_Populated`（source/captured_at/rows 数量 == charge lines 数量、rows[0].price_version）
   - `TestPricer_ChargeLines_CarryPriceSourceAndVersion`
   - `TestPricer_PriceSnapshot_EmptyRowsWhenNoPricing`
6. `repository/model/postgres_test.go`：`TestFindByModel_ScansPriceVersion`（插入 price_version=7，读回 7）
7. `repository/usage/postgres_test.go`：`TestCreateUsageLog_CachedSource`（'cached' 可落库，验证 CHECK 扩展）
8. `billing/logger_test.go`：断言 charge line 优先使用 per-line PriceSource/PriceVersion

## 实施顺序

1. 迁移 000009 创建并应用到 dev 库与 test 库（`migrate -path migrations -database ... up`；repository 测试依赖真实 PG 与最新 schema）
2. domain + model repo（含测试 6）
3. pricer（含测试 5）
4. logger per-line 回退（含测试 8）
5. 网关 final_chunk + 失败日志（含测试 1-4、7）
6. 全量验证：`go test ./...`、`npm run build`、`npx vitest run`、迁移 up/down 往返

## 风险与兼容

- 现有 `TestHandleStreamingChat_SuccessWithUsage` 预期变红 → 行为变更，必须同步更新
- 前端不依赖 usage_source 值，无影响
- evidence/charge_lines 字段向后兼容（新增字段、回退逻辑）
- 失败日志全部走 detached context，避免 client disconnect 导致日志丢失
- reviewer 重点关注：scanner 错误分类（context.Canceled vs io.ErrUnexpectedEOF）、快照 decimal 字符串化、CHECK 约束回滚顺序

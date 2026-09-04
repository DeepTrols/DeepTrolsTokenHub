# TH-P1-CW-01 执行日志 — Pending Payment Order Scanner

- 日期：2026-09-04（Sprint 1 Batch 8）
- 状态：DONE
- 业务提交：见 §3
- 依赖：TH-P1-05（DONE）

## 0. 实现方式

待查支付订单扫描器，按 TDD（RED→GREEN）：先写仓储集成测试
`scanner_query_test.go`（5 用例）、工人单测 `scanner_test.go`（6 用例）
与指标断言令三包编译失败，再实现候选查询、资格规则与接线。

设计决策：

1. **两层分工**：
   - 仓储层 `ListPendingCandidates(ctx, olderThan, limit, channel)`：
     `WHERE status='pending' AND created_at<=$1`（可选
     `AND channel=$n`），`ORDER BY created_at ASC, order_no ASC LIMIT`。
     纯 SELECT，绝不写库；状态/账龄/渠道/批量与确定性排序全部落在
     持久层并经真实 *_test 库验证。
   - 工人层 `internal/worker/paymentscan`：纯函数
     `eligible(o, now)` 执行到期规则（可单测），`Scanner.Run(ctx)`
     每周期只发一条候选查询（默认 minAge=60s、batch=20），错误时
     失败关闭（返回 error + nil 结果，周期记 failed）。
2. **扫描器资格规则（Documentation Requirement）**：一笔候选必须同时
   满足——(a) 状态仍为 `pending`（paid/closed/refunded 永不入选）；
   (b) 未过期：`now < expires_at`（到期时刻本身视为过期，边界排除）；
   (c) 重试窗口已到：`next_retry_at IS NULL OR next_retry_at <= now`
   （TH-P1-05 元数据；NULL=从未查过=立即可查，恰在重试时刻视为已到期）。
   过期订单的关闭属于 TH-P1-CW-04，扫描器不越权。
3. **只读构造**：扫描器对订单行与钱包零写入——集成测试在扫描前后对
   全行快照（状态/元数据/时间戳逐字节比对），工人单测断言候选行在
   Run 前后不变。提供方查单调用本身不在本任务（CW-02 消费 eligible 集）。
4. **过载风险控制（Risk 条目）**：批量上限 20、最小账龄 60s、60s
   周期 + Redis 租约单实例运行，扫描器自身零提供方调用，「过宽扫描
   压垮查单 API」的风险在构造上被钳制。
5. **可观测性**：新增两个计数器家族
   `payment_order_scan_total{channel}`（检查的候选数）与
   `payment_order_scan_eligible_total{channel}`（通过资格规则的候选数），
   按订单行渠道计数，标签经 `SanitizePaymentRoute` 白名单钳制；
   `payment_scanner` 加入 `AllowedWorkers` 白名单（周期/租约指标复用
   TH-P05-11 基线）。
6. **接线（cmd/worker）**：`runPaymentScanner` — 60s ticker、租约键
   `worker:lease:payment_scanner`（TTL 50s）、经 `runSafely` +
   `withLease` 统一恢复与记录；日志输出 `%d/%d pending orders
   eligible for provider query`。生产接线扫描全部渠道（nil 选择器），
   渠道选择器供渠道级部署按需在构造时使用。

## 1. AC 验证

| AC | 要求 | 验证测试 | 结果 |
|---|---|---|---|
| AC-01 | 超过资格账龄的 pending 订单被返回 | TestListPendingCandidatesSelectsOldPending（持久层：旧单入选、过新排除）+ TestScannerRunFiltersAndPreservesOrder（工人层：到期单入选且保序） | PASS |
| AC-02 | paid/closed/refunded 订单被排除 | TestListPendingCandidatesSelectsOldPending（三种终态行全部排除）+ TestEligibleRules（paid/closed/refunded/过期行） | PASS |
| AC-03 | 重试时间未到的 pending 订单被排除 | TestEligibleRules（future retry → false；retry==now → true；past retry → true） | PASS |

Test Requirements 逐项：

- Unit（资格规则）：TestEligibleRules（9 行表，含两个边界：到期时刻
  本身排除、重试时刻恰好等于 now 入选）+ TestScannerRun* 四用例
  （过滤保序、查询参数、默认小批量、空结果/只读）PASS。
- Integration（Postgres 扫描查询）：5 个 `ListPendingCandidates`
  真实库用例（账龄/状态过滤、确定性排序与 limit 前缀、渠道过滤、
  只读回归）PASS。
- Regression（订单列表不变）：TestListPendingCandidatesDoesNotMutate
  （扫描前后 List 快照：OrderNo/Status/UpdatedAt/元数据逐一相等）+
  paymentorder 包既有 9 用例保持绿。
- Failure Injection（NULL 重试元数据）：
  TestListPendingCandidatesNullRetryMetadata（四条元数据全 NULL 的行
  仍可无扫描错误地入选）+ TestScannerRunSourceErrorFailsClosed
  （仓储错误 → Run 失败、无部分结果）PASS。

## 2. 证据

- RED：`go vet` 显示三包编译失败（`ListPendingCandidates` 方法、
  `paymentscan` 包、`AddPaymentOrderScanned` 均不存在）。
- GREEN：paymentorder 包 14 用例（9 回归 + 5 新增）、paymentscan 包
  6 用例、metrics 与 service/payment 包全绿。
- `go vet ./...` exit 0；`go build ./...` exit 0；`gofmt -l` clean。
- 独立全量回归（TH-P05-13 门控退出码契约）：
  `GATE_LOG_FILE=/tmp/p1-gate/p1-cw-01.log scripts/gate_command.sh go test ./... -count=1`
  → 50 包 `ok`（新增 internal/worker/paymentscan）、0 `FAIL`/panic、
  日志完整（3507 字节），go test 退出码 0。

## 3. 提交

- 业务提交：`0d2ace3`（Batch 8 收口提交，与 TH-P1-AL-02 同批，
  16 files, +1517/−28），`web/**` 与既有未跟踪文档不纳入。

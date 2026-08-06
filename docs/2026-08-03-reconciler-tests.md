# P3.1 — 对账 worker 测试 — 完成

> 日期: 2026-08-03
> 依据: 2026-07-31 全栈评估（P3.1 项，对账 worker 0% 覆盖）
> 方法: 接口化 → TDD → code-reviewer 审查
> 结果: **覆盖率 88.1%**，全量 28 包测试通过

---

## 变更日志

### P3.1 — 对账 worker 测试 — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| 接口化 | 手动 | ✅ pool → dbPool 接口 |
| 测试 | 手动（仿 billing_committer 先例） | ✅ 7 个测试，88.1% |
| Review | ecc:code-reviewer | ✅ **WARNING→修复**（3 HIGH, 1 MEDIUM, 1 LOW） |

---

## 改动

### `internal/worker/reconciliation/reconciler.go`
- `Reconciler.pool` 从 `*pgxpool.Pool` → `dbPool` 接口（Query/Exec/QueryRow）
- 编译期断言 `var _ dbPool = (*pgxpool.Pool)(nil)`
- `createDiff` 返回 error（不再静默吞错）；`uuid.MustParse` → `uuid.Parse`
- `Run` 收集 diff 写入错误，失败则返回 error（报告非 false consistency）
- `json.Marshal` 错误处理

### `internal/worker/reconciliation/reconciler_test.go`
- mockDB/mockRow/mockRows（仿 billing_committer）
- 7 个测试：成功无孤儿、有孤儿、countLogs 失败、createRun 失败、findOrphaned 失败仍完成、createDiff 失败、mock 编译期断言
- **exec SQL 断言**：验证 createRun/completeRun/markRunFailed/createDiff 确实执行

---

## 审查发现与修复

| 级别 | 发现 | 修复 |
|------|------|------|
| HIGH | 测试只断言 error，不验证 DB 操作执行 | ✅ 加 exec SQL 断言 |
| HIGH | createDiff 静默吞错（资金差异未记录） | ✅ 返回 error + Run 收集失败 |
| HIGH | uuid.MustParse 可能 panic 崩溃 worker | ✅ 改 uuid.Parse |
| MEDIUM | mock 无编译期断言 | ✅ 加 `var _ pgx.Rows/pgx.Row/dbPool` |
| LOW | json.Marshal 错误忽略 | ✅ 错误处理 + fallback |

---

## 验证

- ✅ 覆盖率 88.1%（超 80% 门槛）
- ✅ `go build ./...` + `go vet ./...` 通过
- ✅ 全量 `go test -p 1 ./...` 28 包全绿

# P3.2 — 对账 L1（provider_evidence）— 完成

> 日期: 2026-08-03
> 依据: 2026-07-31 全栈评估（P3.2 项，对账完整性从 L0 升 L1）
> 方法: planner → 实现 → code-reviewer 审查
> 结果: **覆盖率 89.8%**，全量 28 包测试通过

---

## 变更日志

### P3.2 — 对账 L1 — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ L0+L1 同周期方案 |
| 实现 | 手动 | ✅ 4 新方法 + Run 扩展 |
| 测试 | 手动 | ✅ 12 测试，89.8% |
| Review | ecc:code-reviewer | ✅ **APPROVE**（0 CRITICAL, 0 HIGH） |

---

## 设计

L0 和 L1 在同一 Run 周期执行，`reconciliation_runs.level='L1'`，报告 JSON 嵌套 L0/L1 两段。

### 新增 diff 类型

| diff_type | severity | 检测内容 |
|-----------|----------|---------|
| `missing_evidence` | warning | completed usage_log 无 provider_evidence |
| `usage_mismatch` | warning | usage_log 与 evidence 的 total_tokens 不一致 |
| `error_mislabel` | **critical** | evidence status>=400 但 usage_log 标 completed（计费了失败调用） |

### 新增方法

| 方法 | SQL 要点 |
|------|---------|
| `countEvidence` | COUNT provider_evidence JOIN usage_logs |
| `findMissingEvidence` | LEFT JOIN，pe.id IS NULL + completed |
| `findUsageMismatch` | JSONB `->>'total_tokens'` 对比，双 >0 才 flag |
| `findErrorMislabel` | JOIN，pe.status_code >= 400 + completed |

### 错误隔离（资金完整性）

- `countLogs` 失败 → markRunFailed（无数据不可靠）
- `countEvidence` 失败 → evidenceCount=-1 继续（区分查询失败与零）
- 任一 finder 失败 → 记日志继续（一个查询故障不阻塞全部）
- `createDiff` 失败 → fail run（未记录的差异 = 假一致性）

---

## 审查发现与修复

| 级别 | 发现 | 修复 |
|------|------|------|
| MEDIUM | 测试 SQL fragment 冲突（`SELECT ul.id::text,` 匹配两查询） | ✅ 改用 COALESCE 独特 fragment |
| LOW | usage_mismatch 排除单侧 NULL | ⚠️ 刻意设计（避免假阳性，走 missing_evidence） |
| LOW | 测试未验证 severity | ⚠️ severity 是硬编码常量，非计算逻辑 |

---

## 验证

- ✅ 覆盖率 89.8%（超 80%）
- ✅ 12 测试通过（7 原有 + 5 L1 + 1 组合）
- ✅ `go build` + `go vet` 通过
- ✅ 全量 `go test -p 1 ./...` 28 包全绿
- ✅ 现有 L0 测试零改动

---

## 备注

- `total_tokens` 是跨 OpenAI/Anthropic/Gemini 的通用 key（usageparser 归一化）
- provider_evidence 可能异步写入，missing_evidence 用 warning（非 critical）
- 对账级别现覆盖 L0 + L1，L2（L0↔L1 内部对账）/ L3（与上游对账）留待后续

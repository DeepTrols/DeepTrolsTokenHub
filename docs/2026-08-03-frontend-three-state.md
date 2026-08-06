# P2.2 — 前端 loading/error/empty 三态补齐 — 完成

> 日期: 2026-08-03
> 依据: 2026-07-31 全栈评估（P2.2 项）
> 方法: planner → 实现 → code-reviewer 审查
> 结果: **TSC + build + 116/116 测试通过**

---

## 变更日志

### P2.2 — 前端三态补齐 — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ 9 页缺口清单 + 2 模式 |
| 实现 | 手动 | ✅ 9 页补齐 |
| 测试 | vitest | ✅ 116/116 通过 |
| Review | ecc:code-reviewer | ✅ **APPROVE**（0 CRITICAL, 0 HIGH） |

---

## 缺口清单（planner 审计）

| 页面 | 类型 | 之前缺口 | 模式 |
|------|------|---------|------|
| Dashboard | 只读 | 无 loading/error | early-return（双 query 合并） |
| CallLogs | 只读 | 无 loading/error | early-return |
| Wallet | 只读 | 无 loading/error | early-return（双 query 合并） |
| ModelMarket | 只读 | 无 loading/error | early-return |
| APIKeys | mutation | 无 loading/error | inline banner |
| Providers | mutation | 无 loading/error | inline banner |
| Tenants | mutation | 无 loading/error | inline banner |
| Policies | mutation | 无 loading/error | inline banner |
| Channels | mutation | 无 loading/error | inline banner |

## 两种模式

**只读页（early-return）**：`if (isLoading) return spinner; if (isError) return error+retry;`
**mutation 页（inline banner）**：表单保持可用，error banner + loading spinner 插在列表区上方。

复用 TanStack Query 的 `isLoading`/`isError`/`error`/`refetch`，无新建公共组件（现有 Tailwind 模式已一致）。

---

## 审查发现与修复

| 级别 | 发现 | 修复 |
|------|------|------|
| MEDIUM | ModelMarket useEffect 缺 `search` 依赖（搜索后新 group 不展开） | ✅ 依赖加 `search` |
| MEDIUM | 5 个 mutation 页 loading 时空状态同显（spinner + 「暂无」同时出现） | ✅ 空状态加 `!isLoading &&` 守卫 |
| LOW | Providers 表单 `error` state 与 query `error` 冲突 | ✅ 重命名 `queryError` |

---

## 验证

- ✅ `tsc -b` 无错误
- ✅ `npm run build` 成功
- ✅ `vitest run` 116/116 通过
- ✅ 无新建依赖

---

## 备注

- Login/Register/Docs 静态 tab 不适用三态（走 auth context / 静态内容）
- UsageStats/QuotaManagement/Reconciliation/Security/Playground/Docs/ModelManagement 此前已有三态，未改

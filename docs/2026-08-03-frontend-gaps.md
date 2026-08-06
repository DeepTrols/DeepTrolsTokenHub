# 前端缺口修复 — total_charged / charge line / 审计日志

> 日期: 2026-08-03
> 依据: 9 项成熟度清单评估（前端缺口 + 前端 bug）
> 结果: **后端 28 包 + 前端 182 测试全绿**

---

## 1. total_charged 永远显示 0.00（前端 bug 修复）

**问题**：后端 `walletResponse` 无 `total_charged` 字段，前端 `Wallet.tsx` 引用它永远显示 0.00。

**修复**：
- 后端 `wallet.go`：`walletResponse` 加 `TotalCharged`，查询 `SUM(amount) WHERE tx_type='charge'`（负值取负）
- 前端无需改（已引用该字段，现在能显示真实累计消费）

## 2. charge line 维度明细（前后端补全）

**问题**：charge_lines 有写无读，CallLogs 只显示单一 cost。

**修复**：
- 后端 `usage.go`：新增 `HandleGetUsageChargeLines`——`GET /api/console/usage/{id}/charge-lines`，校验 usage_log 归属 + 返回每维度明细
- 前端 `CallLogs.tsx`：费用列变可点击按钮，展开显示计费维度表格（维度/数量/单价/小计）

## 3. 审计日志页面（前端补全）

**问题**：后端 `GET /admin/audit` 接口已就绪，但前端无路由无页面。

**修复**：
- `AuditLogs.tsx`（新）：admin 只读表格，展示操作/资源/IP/时间
- `App.tsx`：加 `/admin/audit` 路由
- `AdminLayout.tsx`：侧边栏加「审计日志」导航项

---

## 验证

- ✅ 后端 28 包 `go test -p 1 ./...` 全绿
- ✅ 前端 182/182 测试 + TSC + build 通过

---

## 9 项清单更新

| # | 项目 | 之前 | 现在 |
|---|------|------|------|
| 5 | charge line 维度明细 | 只有后端 | ✅ 前后端闭环 |
| 7 | provider cost / invoice | 前后端都无 | ⬜ 仍未实现 |
| 8 | smoke/canary | 前后端都无 | ⬜ 仍未实现 |
| 9 | OEM 租户端 | 租户端缺失 | ⬜ 仍未实现 |
| — | total_charged bug | 显示 0.00 | ✅ 修复 |
| — | 审计日志页 | 无前端 | ✅ 补全 |

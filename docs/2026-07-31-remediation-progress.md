# 质量加固进度追踪

> 启动: 2026-07-31
> 依据: 2026-07-31 全栈评估
> 方法: Harness 流水线（planner → tdd-guide → code-reviewer → security-reviewer）

---

## 总体计划

| # | 事项 | 层面 | 优先级 | 状态 |
|---|------|------|--------|------|
| P0.1 | Worker billing_committer 测试 | 后端 | P0 | ✅ 完成 |
| P0.2 | 配额接入计费流 | 后端 | P0 | ✅ 完成 |
| P1.1 | JWT → httpOnly cookie | 后端+前端 | P1 | ✅ 完成 |
| P1.2 | 前端数据层（TanStack Query） | 前端 | P1 | ⬜ |
| P1.3 | 前端错误边界 | 前端 | P1 | ⬜ |
| P2.1 | 限流 Redis 化 | 后端 | P2 | ⬜ |
| P2.2 | 前端补 loading/error/empty | 前端 | P2 | ⬜ |
| P3.1 | Worker reconciliation 测试 | 后端 | P3 | ⬜ |
| P3.2 | 对账 L1（provider_evidence） | 后端 | P3 | ⬜ |
| P4.1 | 前端测试补完 | 前端 | P4 | ⬜ |

---

## 变更日志

### P0.1 — billing_committer 测试 — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ 21 用例计划 |
| TDD | ecc:tdd-guide | ✅ **43 tests, 100% coverage** |
| Review | ecc:code-reviewer | ✅ APPROVE — 0 CRITICAL, 0 HIGH, 3 MEDIUM |
| Security | 手动审查 | ✅ 无安全漏洞 |

**产出**：
- 新增 `charger_iface.go` — dbPool + chargerInterface 接口 + 编译期类型断言
- 修改 `committer.go` — 接口化 + 修复并发竞态 bug
- 新增 `committer_test.go` — 43 tests, mockPool/mockRows/mockRow/mockCharger

**已知 MEDIUM（不阻塞，后续清理）**：nil guard / SQL 断言缺口 / 3 个无效测试

---

### P0.2 — 配额接入计费流 — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ 完整计划 |
| TDD | ecc:tdd-guide | ✅ **28 new tests, 80-83% coverage, 0 regressions** |
| Review | ecc:code-reviewer | ✅ APPROVE — 0 CRITICAL, 0 HIGH, 1 MEDIUM |
| Security | 手动审查 | ✅ SQL 注入/双重扣除/并发绕过全部安全 |

**产出（6 新文件 + 3 修改）**：
- `internal/domain/quota.go` — QuotaPool/Allocation/LedgerEntry + Remaining()
- `internal/repository/quota/` — 接口 + Postgres（FOR UPDATE + 幂等 + 事务）
- `internal/service/billing/quota.go` — QuotaChecker（Check→429 / Deduct→best-effort）
- `internal/handler/gateway/chat.go` — 路由后→Reserve 前检查，成功后异步 deduct
- `internal/app/app.go` — 注入 QuotaChecker

**已知 MEDIUM**：goroutine 用 `context.Background()` 无限超时，应加 10s timeout（不阻塞）

---

### P1.1 — JWT → httpOnly cookie — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ 20 步骤 / 7 阶段计划 |
| TDD | ecc:tdd-guide | ✅ **90 frontend + 54 middleware tests, 0 regressions** |
| Review | 手动审查 | ✅ APPROVE — 无问题 |

**产出（8 backend + 12 frontend 文件）**：
- Cookie 基础设施：Set-Cookie httpOnly + clearAuthCookie + CookieConfig
- ConsoleAuth：cookie-first 读取 + Authorization header 回退
- CORS 修复：`*` + credentials → 显式 CORS_ORIGIN 环境变量
- AuthContext：/me 验证 + logout 服务端清除
- 前端清理：移除所有 localStorage/setToken/getToken/getUserRole
- 修复 7 个测试文件中预存的 `Role` 字段缺失 bug

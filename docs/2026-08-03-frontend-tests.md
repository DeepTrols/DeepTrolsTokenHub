# P4.1 — 前端测试补完 — 完成（覆盖率 71.7%）

> 日期: 2026-08-03
> 依据: 2026-07-31 全栈评估（P4.1 项，13 页面无测试）
> 方法: planner → 实现 → code-reviewer 审查
> 结果: **179 测试全过**，行覆盖率 46% → 71.7%

---

## 变更日志

### P4.1 — 前端测试补完 — ✅ 完成（部分达标）

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ 13 页清单 + mock 策略 |
| 实现 | 手动 | ✅ 13 个测试文件 |
| 测试 | vitest | ✅ 116 → 179（新增 63） |
| Review | ecc:code-reviewer | ✅ **WARNING→修复**（2 HIGH + 2 LOW） |

---

## 改动

### 新增测试文件（13 个）
Dashboard、CallLogs、Wallet、UsageStats、QuotaManagement、Reconciliation、Security、ModelMarket、Policies、Tenants、Channels、Login、ModelManagement

每个覆盖：渲染 + 加载 + 数据 + 空态 + 错误态（mutation 页加 1 个创建流程）。

### 修改
- `vitest.config.ts`：coverage `all: true` + include 全部 src（让未测文件进入统计），排除 test 目录
- `ModelMarket.tsx`：**修复 P2.2 引入的 hook 顺序 bug**（early-return 在 useEffect 前 → "Rendered more hooks"）
- `Policies.tsx`：名称/模型ID input 加 `aria-label`（无障碍 + 可测性）

---

## 覆盖率进展

| 指标 | 之前 | 之后 |
|------|------|------|
| 无测试页面 | 13 | 0 |
| 测试数 | 116 | 179 |
| 行覆盖率 | 46%（all:false 假象） | **71.7%** |

## 审查发现与修复

| 级别 | 发现 | 修复 |
|------|------|------|
| HIGH | ModelManagement.tsx 0% 无测试文件 | ✅ 补测试 |
| HIGH | Policies 测试用脆 `getAllByRole()[2]` | ✅ 加 aria-label + 精确查询 |
| MEDIUM | Login 测试揭示 auth.tsx 不检查 login ok | ⚠️ 记录（auth.tsx 应检查 response.ok） |
| MEDIUM | Tenants/Channels/Security 交互未测 | ⚠️ 记录（后续加深） |
| LOW | afterEach restoreAllMocks 冗余 | ⚠️ 无害保留 |
| LOW | coverage exclude 缺 test 目录 | ✅ 补上 |

---

## 验证

- ✅ TSC + build 通过
- ✅ 179/179 测试全过
- ✅ 行覆盖 71.7%（未达 80% 门槛，记录为遗留）

---

## 遗留（记录）

1. **覆盖率 71.7% 未达 80% 门槛**：剩余缺口在 mutation 页深层分支（Tenants/Channels/Security 交互流程、ModelMarket 搜索/tab 切换）。code-reviewer 判定「gap is small and well-understood」，可后续加深。
2. **auth.tsx 不检查 login response.ok**：登录 401 不会抛错（真实缺陷，测试揭示了它）。

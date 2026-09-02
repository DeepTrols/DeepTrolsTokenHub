# TokenHub Sprint Backlog · Task Execution Mode 规范

> 本文件是每一轮任务执行前**必须读取**的执行规范（通用规则沉淀，
> 不随单批任务变化）。单批任务的具体范围以当轮执行指令为准，
> 任务定义以 `docs/tasks/` 阶段任务文件为唯一事实来源。

## 1. 文件结构与读取顺序

```
docs/tasks/
├── README.md                        # 本文件：执行规范（每轮必读）
├── BACKLOG_OVERVIEW.md              # 全局规则：Epic/DoD/依赖链/推荐 Sprint（每轮必读）
├── TASK_INDEX.md                    # 任务状态唯一索引（每轮必读，依赖与状态依据）
├── P0.5_PRODUCTION_SAFETY.md        # 阶段任务定义（执行哪个阶段读哪个）
├── P1_PAYMENT.md
├── P2_ENTERPRISE.md
├── P3_RBAC.md
├── P4_LANDING_PAGE.md
├── P5_PRODUCTION_HARDENING.md
└── execution-logs/                  # 执行日志（每任务一份，完成后生成）
    └── TH-P05-XX.md
```

## 2. 执行前置条件（Dependency Check）

对每个指定 Task，从 `TASK_INDEX.md` 读取 `Dependencies` 与 `Status`：

- 只有 **Status = TODO 且全部 Dependencies = DONE** 才允许执行。
- Dependency 未完成时：**不绕过、不修改依赖、不擅自标记依赖 DONE**。
  该任务保持 TODO，执行结果记 `BLOCKED`，仅在最终报告说明。

## 3. Scope 纪律

- 任务定义以阶段任务文件中的 **Scope / Out of Scope / Acceptance Criteria /
  Dependencies** 为准；执行指令与任务文件冲突时以任务文件为准。
- 禁止执行未列入当轮清单的任务；发现「顺手就能做」的后续任务一律不做。
- 每个 Task 独立完成：Implementation → Tests → AC Verification → Result，
  不混成单一巨型改动；commit 边界尽量与 task 边界一致。

## 4. AC Verification 纪律

- 禁止「代码能编译」即宣布 DONE。每个 AC 必须逐条验证并给出具体证据
  （测试名、命令、观测结果、代码路径、迁移结果、HTTP/指标结果）。
- 禁止以 "Implemented / Looks correct / Should work / Covered" 作为证据。

## 5. 状态更新规则（TODO → DONE）

必须同时满足：

1. Implementation complete
2. All required tests pass
3. All AC PASS
4. Task 范围内无未解决 P0/P1 缺陷

任一 AC FAIL：保持 TODO，不得部分标 DONE。依赖不满足：保持 TODO 并报 BLOCKED。

- **只更新 `TASK_INDEX.md` 中对应任务的 Status**；禁止改动
  ID/Title/Phase/Epic/Type/Priority/Dependencies/Estimate。
- 阶段任务文件无 Status 字段时，不为同步状态批量改文件；
  TASK_INDEX 是当前执行状态的唯一索引。
- 禁止修改 Backlog 设计（Task 定义/Roadmap/Sprint 定义）；
  发现任务本身有问题时报告 `Backlog Definition Issue`，由人工决定。

## 6. 执行中发现问题分类

| 类别 | 处理 |
|---|---|
| A. 当前 Task 内 Bug | 修复 |
| B. 后续 Task 已覆盖 | 不修，记录 `Deferred To: TH-XXXX` |
| C. Backlog 未覆盖 | 不擅自扩 scope；记录 Backlog Gap（描述/严重度/建议阶段/建议依赖），不当轮创建任务 |
| D. 架构与任务假设冲突 | 停止该任务，报告 `Architecture Conflict`，不擅自重构 |

## 7. 资金任务附加检查

资金相关任务必须按需覆盖：余额不足、重复请求、幂等、并发、
DB 失败、部分失败、负余额防护、账本一致性 —— 但只执行当前任务
AC 要求的范围。

禁止：float 金额运算、负预留、静默归零兜底、预留完成前调用 provider、
对账/Worker 路径直接改钱包余额（资金调整唯一路径见
BACKLOG_OVERVIEW「Money Safety Rule」）。

## 8. Git 纪律

- 每轮开始前 `git status` 检查基线；`docs/tasks/` 未入库时先单独提交。
- 不得覆盖/删除/reset/stash/修改用户既有未提交变更。
- 业务代码改动按 task 边界独立提交；`docs/tasks/` 的状态与日志更新
  单独提交，不与业务代码混提。

## 9. 执行日志（每任务一份）

位置：`docs/tasks/execution-logs/TH-P05-XX.md`（任务完成后生成）。
内容必须包含：

1. Task 元信息（ID / Title / Phase / Dependencies）
2. Implementation 摘要（代码路径与关键决策）
3. AC Verification 逐条（AC-XX: PASS/FAIL + Evidence）
4. Tests 明细（命令、结果、覆盖的测试要求类型）
5. Files Changed
6. Database Changes（Migration added / Schema changed / Rollback tested）
7. 发现的问题（Deferred / Backlog Gap / Architecture Conflict）
8. Result: DONE / BLOCKED / FAILED

## 10. 批次收尾报告格式

每轮结束**停止并输出**（不继续下一批）：

- **Execution Summary**：每个指定任务的 DONE / FAILED / BLOCKED
- **AC Verification**：逐任务逐条 + Evidence
- **Tests**：实际执行的命令与结果
- **Files Changed**：按任务分类
- **Database Changes**：Migration added / Schema changed / Rollback tested（YES/NO/N/A）
- **Financial Safety**：Negative balance possible / Provider call before required Reserve possible /
  Ledger inconsistency detected —— 未测项必须写 `NOT TESTED`，不得写成 `NO`
- **Deferred Issues** / **Backlog Gaps** / **Architecture Conflicts**（无则 None）
- **Git Status**：commits created / uncommitted files / 用户既有变更未触碰确认
- **Sprint Batch Result**：PASS / FAIL / PARTIAL
- **Ready for next batch**：YES/NO；YES 时只按 TASK_INDEX 依赖列出新解锁任务，等待人工确认。

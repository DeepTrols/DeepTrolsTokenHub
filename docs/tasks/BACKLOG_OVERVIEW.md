# 智曜TokenHub Sprint Backlog Overview

> 生成日期：2026-09-02（第二轮拆分后同步）
> 范围：P0.5-P5 全部 138 个 Sprint Task，唯一事实来源为 `docs/tasks/` 六个阶段任务文件；
> 本概览与 `TASK_INDEX.md` 均由任务文件程序化重新生成，不沿用第一轮拆分的任何旧统计。
> 本轮只创建/更新 `docs/tasks` 任务文档，不修改业务代码。

## Planning Basis

- 现有总体规划：`docs/规划_平台定位与企业化改造方案.md`
- 生产就绪清单：`docs/PRODUCTION_READINESS.md`
- 部署手册：`docs/DEPLOYMENT.md`
- 当前状态记录：`docs/PROJECT_STATUS.md`
- 代码核查范围：支付服务、企业数据层、RBAC/鉴权、Nuxt 落地页、Worker、审计、钱包与对账链路。

## Epic List

| Phase | Epic | Task Count | Output |
|---|---|---:|---|
| P0.5 | Production Safety Gate | 11 | B5 最大预留/结算兜底、账务不变量测试、资金指标告警、备份恢复、干净环境部署、PayURL 持久化、Worker 观测、支付前安全门 |
| P1 | Payment | 34 | QueryOrder 契约、通道工厂与回调路由、支付宝 7 子任务、微信 7 子任务、补偿 Worker 9 子任务、支付对账与 epay 退场 6 子任务 |
| P2 | Enterprise | 43 | 企业申请/审核、邀请与成员治理、共享钱包全链路、网关企业扣费、成员月度上限、企业账单与控制台 |
| P3 | RBAC | 15 | 权限目录、角色 CRUD 与守卫、RequirePerm 中间件、路由权限矩阵、审计、前端权限收敛 |
| P4 | Landing Page | 12 | Nitro 代理与缓存兜底、定价页/首页真实数据、视觉回归、认证跳转与静态原型退役 |
| P5 | Production Hardening | 23 | 对账复核与显式调整（RVW）、推理遥测（TEL）、Prometheus 指标（MET）、密钥与日志安全（SEC）、压测与故障注入（LOAD） |

Epic Count = 6（每个 Epic 对应一个阶段任务文件；Epic/Feature 不作为 Sprint Task 计数）。

## Global Definition of Done

以下作为全部 Sprint Task 的统一 DoD。适用项未完成时，任务不得标记为 Done。

- 代码已完成并通过 Code Review。
- 单元测试通过。
- 集成测试通过。
- 相关回归测试通过。
- 异常路径已覆盖。
- 幂等已验证（适用时）。
- 并发已验证（适用时）。
- Migration 已验证（适用时）。
- Rollback 或恢复方案已验证（适用时）。
- Metrics 已接入（适用时）。
- Audit Log 已接入（适用时）。
- 日志不得包含密钥、Token、支付敏感信息。
- 文档已同步（适用时）。
- `docs/DEPLOYMENT.md` 已更新（涉及部署/配置时）。
- 无阻塞级 P0/P1 缺陷。

## Phase Task Count

| Phase | Task Count | 拆分形态 |
|---|---:|---|
| P0.5 | 11 | TH-P05-01 … TH-P05-11 |
| P1 | 34 | TH-P1-01…05、AL-01…07、WX-01…07、CW-01…09、RC-01…06 |
| P2 | 43 | TH-P2-01…06、INV-01…06、WAL-01…09、GW-01…07、LIM-01…07、UI-01…08 |
| P3 | 15 | TH-P3-01 … TH-P3-15 |
| P4 | 12 | TH-P4-01 … TH-P4-12 |
| P5 | 23 | RVW-01…07、TEL-01…09、MET-01…03、SEC-01…03、LOAD-01 |
| **Total** | **138** | |

## Priority Summary

| Priority | Count |
|---|---:|
| P0 | 11 |
| P1 | 87 |
| P2 | 40 |
| **Total** | **138** |

## Critical Dependency Chains

以下链路全部取自当前任务文件的真实 Dependencies 字段（已验证无悬空、无循环依赖）。

- 资金安全基线（B5）：`TH-P05-01 -> TH-P05-02 -> TH-P05-03`；`TH-P05-02 -> TH-P05-04 -> TH-P05-05 / TH-P05-11`
- 备份与干净环境（B7/B8）：`TH-P05-06 -> TH-P05-07 -> TH-P05-08`
- 支付前安全门（全后续阶段的入口）：`{TH-P05-03, TH-P05-05, TH-P05-08, TH-P05-10, TH-P05-11} -> TH-P05-09 -> {TH-P1-01, TH-P2-01, TH-P3-01, TH-P4-01, TH-P5-RVW-01, TH-P5-TEL-01, TH-P5-SEC-01}`
- 支付通道骨架：`TH-P05-09 -> TH-P1-01 -> TH-P1-02 / TH-P1-03 -> TH-P1-04 / TH-P1-05`
- 支付宝适配：`TH-P1-03 -> TH-P1-AL-01 -> {TH-P1-AL-02, TH-P1-AL-03 -> TH-P1-AL-04, TH-P1-AL-05} -> TH-P1-AL-06 -> TH-P1-AL-07`
- 微信适配：`TH-P1-03 -> TH-P1-WX-01 -> {TH-P1-WX-02, TH-P1-WX-03 -> TH-P1-WX-04, TH-P1-WX-05} -> TH-P1-WX-06 -> TH-P1-WX-07`
- 丢回调补偿 Worker：`TH-P1-05 -> TH-P1-CW-01 -> TH-P1-CW-02 -> {TH-P1-CW-03, TH-P1-CW-04} -> TH-P1-CW-05 -> {TH-P1-CW-06, TH-P1-CW-07, TH-P1-CW-08} -> TH-P1-CW-09`
- 支付对账与 epay 退场：`{TH-P1-AL-06, TH-P1-WX-06} -> TH-P1-RC-01 -> TH-P1-RC-02 -> TH-P1-RC-03 -> TH-P1-RC-06`；`{TH-P1-AL-07, TH-P1-WX-07} -> TH-P1-RC-04 -> TH-P1-RC-05 -> TH-P1-RC-06`
- 企业入驻与审核：`TH-P05-09 -> TH-P2-01 -> TH-P2-02 -> {TH-P2-03, TH-P2-04, TH-P2-05 -> TH-P2-06}`
- 企业成员治理：`TH-P2-02 -> TH-P2-INV-01 -> TH-P2-INV-02 -> {TH-P2-INV-03, TH-P2-INV-04 -> TH-P2-INV-05 / TH-P2-INV-06}`
- 企业共享钱包：`TH-P2-05 -> TH-P2-WAL-01 -> TH-P2-WAL-02 -> TH-P2-WAL-03 -> {TH-P2-WAL-04, TH-P2-WAL-05 -> TH-P2-WAL-06 / TH-P2-WAL-07} -> TH-P2-WAL-08 / TH-P2-WAL-09`
- 网关企业扣费：`{TH-P2-WAL-03, TH-P2-INV-04} -> TH-P2-GW-01 -> TH-P2-GW-02 -> TH-P2-GW-03 -> {TH-P2-GW-04, TH-P2-GW-05, TH-P2-GW-06} -> TH-P2-GW-07`
- 成员月度上限：`TH-P2-INV-01 -> TH-P2-LIM-01 -> {TH-P2-LIM-02, TH-P2-LIM-03 -> TH-P2-LIM-04 -> TH-P2-LIM-05 -> TH-P2-LIM-06 -> TH-P2-LIM-07}`
- 动态权限：`TH-P05-09 -> TH-P3-01 -> TH-P3-02 -> TH-P3-03 -> {TH-P3-04 -> TH-P3-05 / TH-P3-06, TH-P3-07 -> TH-P3-08 -> TH-P3-09 -> TH-P3-10 / TH-P3-11, TH-P3-12 -> TH-P3-13 / TH-P3-14 / TH-P3-15}`
- 落地页真实数据：`TH-P05-09 -> TH-P4-01 -> {TH-P4-02 -> TH-P4-03 -> TH-P4-04 -> {TH-P4-05 -> TH-P4-06 -> TH-P4-07, TH-P4-08 -> TH-P4-09} -> TH-P4-10, TH-P4-11 -> TH-P4-12}`
- 对账复核资金安全链（Diff → Review Item → Human Review → Explicit Adjustment → Wallet Service → Ledger）：`TH-P05-09 -> TH-P5-RVW-01 -> TH-P5-RVW-02 -> TH-P5-RVW-03 -> {TH-P5-RVW-04 -> TH-P5-RVW-05, TH-P5-RVW-06} -> TH-P5-RVW-07`
- 推理遥测：`TH-P05-09 -> TH-P5-TEL-01 -> {TH-P5-TEL-02 -> TH-P5-TEL-03 -> TH-P5-TEL-04, TH-P5-TEL-05, TH-P5-TEL-06, TH-P5-TEL-07} -> TH-P5-TEL-08 -> TH-P5-TEL-09`
- 指标 / 安全 / 压测：`TH-P05-04 -> TH-P5-MET-01 -> TH-P5-MET-02 / TH-P5-MET-03`；`TH-P05-09 -> TH-P5-SEC-01 -> TH-P5-SEC-02 / TH-P5-SEC-03`；`{TH-P05-09, TH-P5-MET-01} -> TH-P5-LOAD-01`

## Money Safety Rule（资金安全红线）

- 对账 Worker 只允许发现差异并生成 `reconciliation_diff` / `review_item`，
  禁止调用钱包任何资金方法（TH-P5-RVW-02 AC-01 显式断言不调用
  Spend / Adjust / TopUp）。
- 任何钱包余额变动必须来自人工复核后的显式调整命令
  （TH-P5-RVW-04，权限门控 + 状态检查），且必须经由钱包服务的
  带账本调用完成，禁止直接改余额（TH-P5-RVW-05 幂等 + 账本约束）。
- 不存在「对账 Worker 自动修复/直接补扣」类任务；第一轮拆分中的
  「Reconciliation Auto Repair And Undercharge Recovery」已移除。

## Code / Planning Inconsistencies Found

- `docs/PRODUCTION_READINESS.md` 仍将 B2 账外注资标为未修复，但 `docs/PROJECT_STATUS.md` 第 108 节已记录 B2 修复完成；本 Backlog 不重复创建 B2 修复任务，只创建存量历史账务处理与文档同步相关任务。
- `payment_orders.pay_url` schema 与 DTO 已存在，但当前 `paymentorder.Create` 未写入 `pay_url`，导致订单刷新后无法从订单行恢复支付链接（对应 `TH-P05-10`）。
- `docs/规划_平台定位与企业化改造方案.md` 提到企业钱包可复用既有 `wallet.Repository`，但当前代码仍通过 `FindByUser(userID, nil)` 走个人钱包；企业钱包需要明确独立 schema/repository 或 holder 抽象后再接入网关（对应 `TH-P2-WAL-01` 设计记录）。
- `ai-nuxt/server/api/*.get.ts` 当前返回本地 JSON 快照；后端真实公开接口已经存在，落地页仍未接入真实数据源（对应 P4 全部任务）。
- 后台权限仍以 `users.role=user|admin` 和 `AdminAuth()` 为主，规划中的动态 RBAC 尚未进入 schema/API/前端（对应 P3 全部任务）。
- Worker 已有 Redis lease，但支付丢回调补偿 Worker 尚不存在；不能把现有对账 Worker 视为支付主动查单能力（对应 `TH-P1-CW-*`）。
- Public stats 只返回 routable model count；若首页需要更多转化数据，需要按脱敏聚合接口增量定义字段。

## Recommended Sprint 1（P0.5 Money Safety Foundation）

Sprint 1 只做 P0.5：支付与真实收款开始前必须通过的资金安全基线。
不启动任何 Alipay / WeChat provider 实现、企业实现或 RBAC 实现
——它们全部依赖 `TH-P05-09` 安全门通过。

- `TH-P05-01` B5 最大费用预留计算
- `TH-P05-02` B5 结算兜底可见性修正
- `TH-P05-03` 账务不变量与并发测试矩阵
- `TH-P05-04` 网关计费基础指标
- `TH-P05-05` 生产基础告警
- `TH-P05-06` 数据库备份基线
- `TH-P05-07` 备份恢复演练
- `TH-P05-08` 干净环境部署验证
- `TH-P05-10` 支付订单 PayURL 持久化修复
- `TH-P05-11` Worker Lease 可观测
- `TH-P05-09` 生产安全门 Harness（收口，依赖以上全部证据）

目标：B5 治理 + 账务不变量 + 指标告警 + 备份恢复 + 干净部署全部有证据，
`TH-P05-09` 安全门通过后才解锁 P1 支付、P2 企业、P3 RBAC、P4 落地页、
P5 硬化各阶段的首个任务。

## Recommended Sprint 2（P1 支付骨架，安全门通过后）

仅包含依赖图上被 Sprint 1 直接解锁的任务：

- `TH-P1-01` QueryOrder 结果契约
- `TH-P1-02` QueryOrder 结算意图服务
- `TH-P1-03` 支付通道工厂
- `TH-P1-04` 回调路由通道解析
- `TH-P1-05` 支付订单 Provider 元数据
- `TH-P1-AL-01` 支付宝配置与启动校验
- `TH-P1-WX-01` 微信支付配置与启动校验

目标：通道选择、回调路由与订单元数据就位，双官方渠道完成配置级接入，
为后续 CreateOrder / Notify / QueryOrder 子任务铺路。

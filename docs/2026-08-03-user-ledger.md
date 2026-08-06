# 用户账本功能 — 完成

> 日期: 2026-08-03
> 依据: 用户反馈「应该能看到每个用户账号的金额和使用量」
> 结果: **后端 28 包 + 前端 182 测试全绿**

---

## 背景

作为平台运营方，需要看每个用户的账本：充多少、花多少、剩多少、调用了多少次。此前用户列表只有基础字段（email/role/status），无财务和使用数据。

## 实现

### 后端 `internal/handler/console/ledger.go`
- `HandleUserLedger`：`GET /api/admin/ledger`
- 一次查询聚合每个用户：
  - `balance`（当前可用余额，来自 wallets）
  - `total_topup`（累计充值，wallet_transactions 的 topup 汇总）
  - `total_spend`（累计消费，usage_logs 的 final_cost 汇总）
  - `request_count`（调用次数）
  - `total_tokens`（累计 token，usage_raw JSONB 提取）

### 前端 `web/src/pages/Users.tsx`
- 改用 `/ledger` 端点，标题改为「用户账本」
- 表格列：用户 / 角色 / 余额 / 累计充值 / 累计消费 / 调用次数 / Tokens / 操作
- 保留封禁/授权操作

### 路由
- `cmd/api/main.go`：`GET /admin/ledger`

---

## 修复的 SQL 问题

`wallet_transactions` 无 `user_id` 列（只有 `wallet_id`），topup 子查询需通过 `JOIN wallets` 关联到用户。

---

## 验证

- ✅ 后端 28 包全绿
- ✅ 前端 182/182 测试 + TSC + build 通过
- ✅ 真实 API 冒烟：返回每个用户的余额/消费/调用/token（admin 21 调用 3205 tokens 等真实数据）

---

## 运营价值

现在运营方能回答：
- 谁充了钱、剩多少（余额）
- 谁在消耗（调用次数、tokens）
- 谁是高价值客户（消费高）
- 谁是流失客户（注册后 0 调用）

# 钱包充值功能 — 完成

> 日期: 2026-08-03
> 依据: 核心业务流程审计（CRITICAL：赠金耗尽后账户成死账户）
> 结果: **全量 28 包测试通过**

---

## 问题

用户只能靠注册时的硬编码赠金（1000 CNY）+ admin 的 10000 CNY，**无任何充值方式**。赠金耗尽后账户成死账户，无法商业化。

## 修复

### `internal/repository/wallet/repository.go`
- Repository 接口新增 `TopUp(ctx, walletID, amount, idempotencyKey)`

### `internal/repository/wallet/postgres.go`
- 实现 `TopUp`：事务锁行 → 校验金额 > 0 → 幂等检查 → 加余额 + version → 记录 `topup` 交易
- 乐观锁（`WHERE version = $N`）防并发冲突

### `internal/handler/console/wallet.go`
- 新增 `HandleTopUp` handler：金额用 **decimal** 解析（禁 float）→ 找钱包 → 调 TopUp（幂等）→ 返回交易详情
- 幂等键从 `X-Idempotency-Key` / `X-Request-ID` 头取，防重复充值

### `cmd/api/main.go`
- 新增路由 `POST /api/console/wallet/topup`

### 测试
- `postgres_test.go` 新增 `TestWalletTopUp`（4 用例：充值+交易、幂等、拒绝非正金额、未知钱包）
- gateway/billing 测试 mock 补 `TopUp` 方法满足接口

---

## 资金安全（符合项目不变量）
- ✅ 金额计算全用 decimal，禁 float
- ✅ 幂等键防重复充值
- ✅ 事务 + 乐观锁防并发
- ✅ 充值记录 `topup` 交易，balance_after 可追溯

---

## 验证

- ✅ `go build ./...` + `go vet ./...` 通过
- ✅ 全量 `go test -p 1 ./...` 28 包全绿
- ✅ 新增 4 个 TopUp 测试全过

---

## 前端充值 UI（追加）

### `web/src/pages/Wallet.tsx`
- 页面顶部新增「充值」按钮 + 充值表单（金额输入 + 确认按钮）
- 用 `useConsoleMutation("post", "/wallet/topup", "/wallet")` 调后端接口
- 充值成功后 invalidate `/wallet` + refetch transactions
- 金额校验（>0）+ 错误提示

### `web/src/pages/Wallet.test.tsx`
- 新增 2 个充值测试（成功调接口、非法金额拦截）
- 适配「充值」文本重复断言

**前端 182/182 测试通过，build 通过。**

---

## 遗留（记录）

- 退款/转账 handler 仍缺失（domain 常量已定义）

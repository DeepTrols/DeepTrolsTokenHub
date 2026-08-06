# TOTP 登录断链修复 — 完成

> 日期: 2026-08-03
> 依据: 核心业务流程审计（CRITICAL：启用 MFA 的用户被永久锁死）
> 结果: **180/180 测试通过**，TSC + build 通过

---

## 问题

**启用 TOTP 的用户无法登录。**

- 后端：`auth.go:98` 检测到 TOTP 用户返回 `{"error":"TOTP code required","mfa_required":"true"}` (401)
- 前端：`auth.login(email, password)`：
  1. 不检查 `res.ok`（登录失败不抛错）
  2. 无 `totp_code` 参数
  3. 不处理 `mfa_required` 响应
- **用户影响**：登录显示「登录失败」，永远无法完成 MFA 认证 → 被锁死

## 修复

### `web/src/lib/auth.tsx`
- `login` 签名改为 `(email, password, totpCode?)`，返回 `LoginResult`（success/mfaRequired/error）
- 检查 `res.ok`；401 时读 `mfa_required` 判断是否需两步验证
- 网络错误返回「网络错误，请稍后重试」

### `web/src/pages/Login.tsx`
- 新增 `mfaRequired` state + TOTP 验证码输入框
- 第一步登录：若 `mfaRequired` 则显示验证码输入
- 第二步：带 `totpCode` 重新登录 → 成功后导航

### `web/src/pages/Login.test.tsx`
- 适配 login 新签名
- 新增 TOTP 两步流程测试（输入验证码 → 登录成功导航）

---

## 验证

- ✅ TSC + build 通过
- ✅ 180/180 测试通过（新增 1 个 TOTP 流程测试）
- ✅ 启用 MFA 的用户现在可正常两步登录

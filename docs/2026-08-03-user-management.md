# 用户管理功能 — 完成

> 日期: 2026-08-03
> 依据: 用户反馈「最重要的一点是用户管理」（此前完全缺失）
> 结果: **后端 28 包 + 前端 182 测试全绿**

---

## 背景

用户管理此前**完全空白**——只有注册/登录/me，无用户列表、无封禁、无授权、无改密、无资料编辑。这是平台运营的地基。

## 后端

### `internal/repository/user/repository.go`
- Repository 接口新增 6 方法：`List`/`UpdateStatus`/`UpdateRole`/`UpdateProfile`/`UpdatePassword`/`Count`

### `internal/repository/user/postgres.go`
- 实现上述 6 方法（分页查询、状态/角色/资料/密码更新、计数）

### `internal/handler/console/users.go`（新增 5 handler）
| handler | 权限 | 用途 |
|---------|------|------|
| `HandleListUsers` | admin | 用户分页列表 + 总数 |
| `HandleUpdateUserStatus` | admin | 封禁/解封/删除（防自改） |
| `HandleUpdateUserRole` | admin | 授权/降权 |
| `HandleUpdateProfile` | 自助 | 改昵称 |
| `HandleChangePassword` | 自助 | 改密（验证旧密码 + bcrypt） |

### `cmd/api/main.go`
- admin 路由：`GET /users`、`PUT /users/{id}/status`、`PUT /users/{id}/role`
- console 路由：`PUT /me/profile`、`PUT /me/password`

## 前端

### `web/src/pages/Users.tsx`（新增）
- admin 用户管理页：列表 + 封禁/解封 + 授权/降级 + 2FA 状态
- 三态（加载/错误/空）+ mutation 操作

### `web/src/App.tsx` + `AdminLayout.tsx`
- 新增 `/admin/users` 路由 + 「用户管理」导航项

---

## 验证

- ✅ 后端 28 包 `go test -p 1 ./...` 全绿
- ✅ 前端 182/182 测试 + TSC + build 通过
- ✅ 真实 API 冒烟：登录 → 用户列表（返回数据）→ 修改资料（updated）

---

## 遗留（记录）

- 找回密码（需邮件基础设施，后置）
- 用户详情页（当前列表够用）
- 前端个人设置页（profile/password 接口已就绪，可后续在「安全设置」页加）

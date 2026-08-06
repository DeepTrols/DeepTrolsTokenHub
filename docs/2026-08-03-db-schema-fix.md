# DB Schema 修复 — encrypted_key 迁移 — 完成

> 日期: 2026-08-03
> 依据: P2.1 期间发现的预存问题（api_keys 缺 encrypted_key 列）
> 方法: code-explorer 审计 → 补迁移 → 修复扫描
> 结果: **所有 repository + gateway handler 测试通过**

---

## 变更日志

### DB schema 修复 — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| 审计 | ecc:code-explorer | ✅ 唯一脱节：`api_keys.encrypted_key` |
| 迁移 | 手动 | ✅ 000003 新增 |
| 修复 | 手动 | ✅ COALESCE 扫描 + user 测试 seed 补 role |

---

## 审计结论

code-explorer 全面比对 23 张表的 migration 定义 vs 代码 SQL 引用：

| 表 | 状态 |
|----|------|
| `api_keys` | ❌ 缺 `encrypted_key` 列（唯一脱节） |
| 其余 22 表 | ✅ 全部干净 |

**数据库实际已有 `encrypted_key` 列**（手动建库或旧脚本），但 migration 文件落后 → 新环境跑 migrate 会失败。

---

## 修复内容

### 1. 新增迁移 `migrations/000003_add_encrypted_key.{up,down}.sql`

```sql
-- up
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS encrypted_key TEXT;
-- down
ALTER TABLE api_keys DROP COLUMN IF EXISTS encrypted_key;
```

幂等 `IF NOT EXISTS`：已有该列的库安全跳过。已应用到主库 + 测试库。

### 2. 修复 NULL 扫描崩溃（`internal/repository/apikey/postgres.go`）

`encrypted_key` 列允许 NULL，但 `scanKey` 扫描到 `string` 会报 `cannot scan NULL into *string`。SELECT 用 `COALESCE(encrypted_key, '')` 处理。

### 3. 修复 user 测试 seed 缺 role（`internal/repository/user/postgres_test.go`）

`000002_add_role` 迁移给 `users.role` 加 `NOT NULL` + CHECK 约束，但测试 seed 的 `domain.User{}` 未填 `Role`（空串违反约束）。补 `Role: "user"`。

---

## 验证

- ✅ `go build ./...` + `go vet ./...` 通过
- ✅ **所有 repository 测试**（apikey/user/usage/channel/model/quota/tenant/wallet）串行通过
- ✅ **gateway handler** 测试通过
- ✅ middleware / ratelimit / app 包测试通过
- ⚠️ repository 并行测试有竞争（共享测试库 TRUNCATE）→ 需 `-p 1`（Makefile `test-repo` 已处理）

---

## 遗留项修复（追加 2026-08-03）

全量 `go test -p 1 ./...` 28 包全部通过。

### ✅ console handler 契约漂移（已修复）
- `HandleListModels` 新增 `pricing` map 字段（保留 `pricings` 数组），同时满足前端 ModelMarket（map）+ ModelManagement（数组）+ 测试
- `HandleListProviders`：`name` 改为取 `ch.name`（友好名）；`JOIN` → `LEFT JOIN` 支持无实例 channel；扫描改 `*time.Time`/`*string` 处理 NULL

### ✅ TestHandleCreateProvider_* 外网依赖（已修复）
- `discoverModels` 改为可注入的包级变量 `discoverModelsFn`，测试 stub 避免外网调用
- 更新过时断言：NoModelsExist 期望 201（凭证总能保存）、Success/WithDefaultBaseURL 验证 DB 而非响应 id
- 修正 deepseek base_url 期望（无 /v1，executor 运行时拼接）

### ✅ scripts TestDelAdmin 副作用 + FK 顺序（已修复）
- 改为事务 + 回滚（不再删除主库真实 admin 数据）
- FK 安全顺序 + TRUNCATE CASCADE 处理深层依赖链

### ⚠️ repository 并行测试竞争（预存，非本次）
多个 repository 包并行连同一测试库互相 TRUNCATE 冲突，需 `-p 1`（Makefile `test-repo` 已处理）。

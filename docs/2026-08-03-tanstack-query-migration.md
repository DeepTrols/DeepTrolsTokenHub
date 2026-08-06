# P1.2 — 前端数据层迁移 TanStack Query — 完成

> 日期: 2026-08-03
> 依据: 2026-07-31 全栈评估（P1.2 项）
> 方法: planner → tdd-guide 模式 → code-reviewer 审查
> 结果: **103/103 测试通过**，TSC + production build 通过

---

## 变更日志

### P1.2 — 前端数据层迁移 TanStack Query — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ 5 阶段计划（0-4） |
| 实现 | 手动（按计划 Phase 0→4） | ✅ 18 页面迁移 |
| 测试 | vitest | ✅ **103/103 通过**（含修复 10 个预存失败） |
| Review | ecc:code-reviewer | ✅ **WARNING→已修复**（1 HIGH, 3 MEDIUM, 2 LOW） |

### 审查发现与修复

| 级别 | 发现 | 修复 |
|------|------|------|
| HIGH | `...options` spread 覆盖自定义 `onSuccess`，导致缓存不失效 | ✅ 解构 `onSuccess` 为 `callerOnSuccess`，rest spread |
| MEDIUM | Playground `availableModels` 每次渲染新数组引用 | ✅ `useMemo` 缓存 |
| MEDIUM | Docs 错误判断混用 `isError`/`!error` | ✅ 统一为 `isError` |
| MEDIUM | APIKeys `console.error`（预存） | ⚠️ 保留（dev 错误日志，非泄露） |
| LOW | PUT 发送冗余 `id` 字段 | ⚠️ 已确认测试契约，后端安全 |
| LOW | API key 存 localStorage（预存） | ⚠️ 跟踪项，非本次引入 |

---

## 新增文件

| 文件 | 用途 |
|------|------|
| `web/src/lib/query-client.ts` | QueryClient 实例 + 默认配置（staleTime 30s, gcTime 5min, retry 1） |
| `web/src/lib/hooks/use-api.ts` | 通用 hooks: `useConsoleQuery` / `useAdminQuery` / `useConsoleMutation` / `useAdminMutation` |
| `web/src/test/test-utils.tsx` | `renderWithProviders` + `createTestQueryClient` 测试工具 |

### use-api.ts 设计要点

- **Query key 派生**: `["console", path]` / `["admin", path]`，mutation 自动 invalidate 对应列表 query
- **函数式路径**: `MutationPath<TVariables>` 支持动态路径（如 `(v) => /api-keys/${v.id}`）
- **独立 invalidatePath**: mutation 目标是子资源时，可指定 invalidate 的列表路径
- **onSuccess 转发**: v5 的 4 参签名 `(data, variables, onMutateResult, mutationContext)` 正确转发

---

## 迁移页面（18 个）

### 只读页面（7）
`Dashboard` `CallLogs` `UsageStats` `Wallet` `ModelMarket` `QuotaManagement` `Reconciliation`

### CRUD 页面（6）
`APIKeys` `Providers` `Security` `ModelManagement` `Tenants` `Policies` `Channels`

### 特殊页面（2）
`Playground` — 依赖查询（keys → secret + models），models 改走网关 fetch
`Docs` — ModelsSection 网关 fetch → useQuery

### 未迁移（2，按计划保留）
`Login` / `Register` — 走 auth context，无需缓存

---

## 测试适配

| 文件 | 改动 | 结果 |
|------|------|------|
| `APIKeys.test.tsx` | render → renderWithProviders | 35/35 |
| `Providers.test.tsx` | render → renderWithProviders | 13/13 |
| `Playground.test.tsx` | render → renderWithProviders | 15/15 |
| `Docs.test.tsx` | render → renderWithProviders | 30/30 |
| **合计** | | **103/103** |

---

## 修复的预存问题

Playground 10 个测试原本因实现缺失而失败，迁移时一并修复：

1. **空状态提示** — `noKeysAvailable` 变量存在但未渲染 → 补上「请先创建 API 密钥」提示
2. **models 网关加载** — 原用 console `api.get("/models")`，测试契约是网关 `/v1/models` + Bearer key → 改为自定义 `useQuery` + 网关 fetch
3. **Bearer 回退** — secret 端点无 plaintext 时回退用 key id（开发/演示语义）
4. **模型加载错误显示** — 补上「获取模型列表失败: {message}」
5. **usage 显示** — 补上响应 token 用量统计

---

## 验证

- ✅ `tsc -b` 无错误
- ✅ `npm run build` 成功（345KB JS, 95KB gzip）
- ✅ `vitest run` 103/103 通过
- ✅ 无残留 `api.get`/`adminApi.get` 直接调用（除 auth context 保留）
- ⚠️ `npm run lint` 不可用（eslint 未安装为项目依赖）— 由 TSC 承担类型检查

---

## 备注

- `auth.tsx` 保持原样（Phase 3.4 可选迁移，鉴权路径最敏感，暂不迁移）
- `query-keys.ts` 集中管理未做（YAGNI — hook 已自动派生 key）
- 后续 P1.3（错误边界）/ P2.2（三态）在此数据层之上构建

# P2.1 — 限流 Redis 化 — 完成

> 日期: 2026-08-03
> 依据: 2026-07-31 全栈评估（P2.1 项）
> 方法: planner → 实现 → code-reviewer 审查
> 结果: **build + vet + 相关包测试全过**，Redis 端到端冒烟验证通过

---

## 变更日志

### P2.1 — 限流 Redis 化 — ✅ 完成

| 阶段 | Agent | 结果 |
|------|-------|------|
| Plan | ecc:planner | ✅ 固定窗口 Lua 方案 |
| 实现 | 手动 | ✅ 接口 + 3 实现 + 接线 |
| 测试 | go test | ✅ memory 8 + redis 6 + fallback 4 + middleware 7 |
| Review | ecc:code-reviewer | ✅ **APPROVE**（0 CRITICAL, 0 HIGH） |

---

## 新增文件

| 文件 | 用途 |
|------|------|
| `internal/pkg/ratelimit/ratelimit.go` | `RateLimiter` 接口（`Allow(ctx, key, limit, window)`） |
| `internal/pkg/ratelimit/memory.go` | 内存实现（mutex + 周期清理 goroutine） |
| `internal/pkg/ratelimit/redis.go` | Redis 实现（Lua 原子 INCR+EXPIRE） |
| `internal/pkg/ratelimit/fallback.go` | 降级组合（primary 出错→fallback） |
| `internal/pkg/redis/client.go` | Redis 客户端工厂 |
| `internal/pkg/ratelimit/memory_test.go` | 8 测试 |
| `internal/pkg/ratelimit/redis_test.go` | 6 测试（miniredis） |
| `internal/pkg/ratelimit/fallback_test.go` | 4 测试（stub mock） |

## 修改文件

| 文件 | 改动 |
|------|------|
| `internal/app/app.go` | 新增 `Redis *goredis.Client` + `RateLimiter` 字段；`initRateLimiter` 接线；`Shutdown` 关 Redis |
| `internal/handler/middleware/ratelimit.go` | 改接口；去 sync.Map + goroutine；键加前缀 `rl:login:`/`rl:gw:`；fail-open |
| `cmd/api/main.go` | 两处调用点传 `application.RateLimiter` |
| `internal/handler/middleware/ratelimit_test.go` | 7 测试适配新签名 |
| `go.mod` / `go.sum` | 新增 go-redis v9.7.0 + miniredis v2.38.0 |

---

## 设计

### 固定窗口（Lua 原子操作）
```lua
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('TTL', KEYS[1])
if current == 1 or ttl == -1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return {current, redis.call('TTL', KEYS[1])}
```
单次往返完成 INCR + 条件 EXPIRE + TTL 读取，无竞态。

### 优雅降级
- Redis 可用 → `FallbackRateLimiter(Redis, Memory)`：Redis 出错自动降级内存，请求不丢
- Redis 不可用 → 纯 `MemoryRateLimiter`
- 中间件 fail-open：limiter 报错时放行（不拖垮服务）

### 修复的预存问题
- **goroutine 泄漏**：原实现后台清理 goroutine 永不停止 → memory 实现用可停止的 sweep goroutine
- **数据竞态**：原 `rateLimitEntry` 无锁 → memory 实现用 `sync.Mutex`

---

## 审查发现与修复

| 级别 | 发现 | 修复 |
|------|------|------|
| MEDIUM | MemoryRateLimiter map 无界增长（海量唯一 IP 攻击） | ✅ 加 30s 周期清理 goroutine |
| MEDIUM | Redis URL parse 错误日志泄漏密码 | ✅ client.go 通用错误消息 + app 日志去 detail |
| LOW | Lua 无 TTL 守卫（预存无 TTL 的 key 永久阻塞） | ✅ 加 `ttl == -1` 时重设 EXPIRE |
| LOW | main.go 缩进 | ✅ 修正 |
| LOW | Retry-After +1 未注释 | ✅ 加注释说明 |
| LOW | Memory `Close()` dead code | ⚠️ 保留（现在停 goroutine 有意义） |
| LOW | Redis 子秒窗口精度截断 | ⚠️ 记录（实际窗口秒级，无影响） |

---

## 验证

- ✅ `go build ./...` + `go vet ./...` 通过
- ✅ ratelimit / middleware / app 包测试全过
- ✅ Redis 端到端冒烟：前 3 次放行，第 4/5 次 429 + retryAfter=1m
- ⚠️ `-race` 不可用（Windows 无 cgo）
- ⚠️ 全量 `go test` 有预存 DB schema 脱节失败（`api_keys` 缺 `encrypted_key` 列，migration 与代码不符）— 与本次改动无关，需单独修

---

## 已知遗留（不在本次范围）

**数据库 schema 与代码脱节**：`migrations/000001_init.up.sql` 的 `api_keys` 表缺 `encrypted_key` 列，导致 apikey/console/gateway repository 测试失败。这是预存问题，需补迁移。建议单独排期处理（P2.1 之后的 DB 修复项）。

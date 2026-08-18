# DeepTrols 生产部署手册

> 目标读者：负责把 DeepTrols 部署到生产环境的 SRE / 后端工程师。
> 本文是发布清单（runbook），不是架构文档。架构见 `docs/AI聚合网关_完整文档.md`。

## 0. 上线前置条件（必须全部满足）

- [ ] 已接入真实支付（`ENABLE_FAKE_PAYMENT=false` 时演示充值/兑换码/注册送余额全部关闭）。若尚未接入支付，则只允许内部 Beta，不允许公开收费。
- [ ] 生产配置基线通过启动校验：config 在 `ENABLE_FAKE_PAYMENT=false` 时强制
      `COOKIE_SECURE=true`、`ADMIN_PASSWORD` ≥ 12 字节、所有密钥非弱默认值，
      不满足则拒绝启动（fail-fast）。
- [ ] 至少一个实例的 Worker 已启用（健康检查与对账）。多实例部署必须配合 Redis
      lease（见「多实例注意事项」）。
- [ ] TLS 在反向代理 / 云 LB 终止，后端只监听内网回环。

## 1. 环境变量基线

| 变量 | 生产要求 | 生成方式 |
|---|---|---|
| `DATABASE_URL` | 强密码 + SSL（`sslmode=require` 或云厂商 CA） | 数据库控制台 |
| `REDIS_URL` | 启用密码，走内网/TLS | `redis-cli CONFIG SET requirepass ...` |
| `LITELLM_BASE_URL` | 内网地址；勿把 LiteLLM 暴露公网 | — |
| `LITELLM_MASTER_KEY` | ≥ 32 字节强随机 | `openssl rand -hex 32` |
| `JWT_SECRET` | ≥ 32 字节强随机，泄露=全站会话伪造 | `openssl rand -hex 32` |
| `ENCRYPTION_KEY` | 恰好 32 字节强随机，泄露=全部 API Key 明文暴露 | `openssl rand -hex 16` |
| `ADMIN_PASSWORD` | ≥ 12 字节强密码，首登后立即轮换 | 密码管理器生成 |
| `COOKIE_SECURE` | `true`（强制） | — |
| `COOKIE_SAMESITE` | `Strict`（默认） | — |
| `ENABLE_FAKE_PAYMENT` | `false`（强制） | — |
| `CORS_ORIGIN` | 仅前端正式域名 | — |

生产校验由 `internal/config/config.go` 的 `validate()` 执行；任何不满足的配置都会在进程启动时报错退出，部署流水线应把「启动即退出」视为失败。

## 2. 构建与启动

```bash
go build -trimpath -ldflags "-s -w" -o bin/api ./cmd/api
go build -trimpath -ldflags "-s -w" -o bin/worker ./cmd/worker

# 以 systemd / 容器管理工具托管，至少一个 worker 进程
./bin/api &
./bin/worker &
```

健康检查：

- 存活探针：`GET /health`（200 且 `{"status":"ok"}`）
- 就绪探针：`GET /readyz`（校验 DB / Redis / LiteLLM 连通性；未实现时以 `/health` 代替，并尽快补上）

## 3. 数据库迁移

```bash
# 升级（先备份）
migrate -path migrations -database "$DATABASE_URL" up

# 回滚到上一版本（仅在发布回滚场景使用）
migrate -path migrations -database "$DATABASE_URL" down 1
```

**dirty 版本修复（事故处理）**：

1. 先确认 dirty 版本号：`migrate -path migrations -database "$DATABASE_URL" version`
2. 确认该版本的 up 语句是否已真正执行（查询对应表结构 / 数据）。
3. 若已执行：`migrate -path migrations -database "$DATABASE_URL" force <version>`
4. 若未执行完：先手工补齐缺失 DDL（或回滚已执行的半截），再 `force`。
5. 记录事故原因到 `docs/PROJECT_STATUS.md`。

> 历史事故：2025 年曾因并发/中断出现 `schema_migrations` dirty，靠
> `migrate force 8` 现场修复。迁移操作必须串行执行、避开流量高峰。

## 4. 密钥轮换

- `JWT_SECRET`：滚动发布（新值生效后旧 token 全部失效，通知用户重新登录）。
- `ENCRYPTION_KEY`：当前实现不支持多密钥解密，轮换会破坏已存储的 API Key。
  如需轮换，必须提供重新加密数据的迁移工具后再操作。
- `LITELLM_MASTER_KEY` / 各渠道 `api_key`：在下游渠道侧轮换后更新库内配置。

## 5. 备份与恢复

```bash
pg_dump "$DATABASE_URL" -Fc -f deeptrols_$(date +%F).dump
```

- 备份频率：至少每日；对账/计费库建议 WAL 归档。
- 恢复演练：每季度在隔离环境执行一次恢复验证。
- 备份必须异地/对象存储留存，且加密。

## 6. 多实例注意事项

- Worker（健康检查 / 对账）**必须**启用 Redis lease（`internal/pkg/lease`）；
  否则两个 worker 实例会重复执行对账、产生冲突结果。
- API 无状态，可水平扩展；Redis 与 PostgreSQL 是共享状态。

## 7. 灰度与回滚

- 灰度：API 新版本先在 1 个实例验证 `/health`、登录、一次真实网关调用，
  观察错误率与计费日志后再全量。
- 回滚：二进制回退到上一发布版本；若涉及新迁移，按第 3 节 `down 1` 回滚。
- 任何发布必须先在 staging 跑一遍 `go test -p 1 ./...` 与迁移 up/down 往返。

## 8. 上线后第一周观察项

- 对账 L0/L1 跑批无差异、无残留 pending。
- 非流式/流式失败请求全部能在 `usage_logs` 找到（`status=failed/partial`），
  不存在"账外请求"。
- 4xx 错误率、网关延迟 P95、Redis 命中率、DB 连接数处于基线内。
- 若发现异常，按 `docs/PROJECT_STATUS.md` 的已知问题章节排查。

# DeepTrols 生产就绪检查清单

> 生成日期：2026-08-19
> 复核日期：2026-08-31（对照当前代码同步状态，详见 PROJECT_STATUS.md 九十八节）
> 来源：全仓代码审计 + 开发库真实数据核对（usage_logs / wallet_transactions / model_pricing 三角校验）
> 定位：判断"能否对外收费上线"的检查清单，按 blocker → 尽快 → 增强排序。
> 状态图例：⬜ 未开始 · 🟡 部分/进行中 · ✅ 已完成

---

## 〇、已具备的基础（不需要重做）

| 能力 | 状态 | 说明 |
|---|---|---|
| 计费引擎（Reserve-Commit-Release） | ✅ | 乐观锁 + 幂等 + decimal，锁行防超扣，账本数学验证正确 |
| 5 个不变量 | ✅ | 复合账务身份 / 预算先预留 / 路由进证据链 / usage 来源标记 / 流式错误不伪装成功 |
| 证据链 | ✅ | usage_log + charge_line + provider_evidence 三表事务；价格快照带版本与来源 |
| 鉴权安全基线 | ✅ | API Key HMAC 哈希、JWT + 登录会话管理（jti，列表/撤销）、租户 fail-closed、安全头、生产 fail-fast |
| 可观测性 | ✅ | slog 结构化日志、请求日志中间件、/healthz + /readyz |
| 测试基建 | ✅ | 每包独立 schema 并行测试（~60-80s）、CI 含 -race |
| 限流/网关边界 | ✅ | Redis 优先 + 内存降级；网关按 key 6 边界 + RPM/TPM 分钟桶 |

---

## 一、上线前必须解决（Blockers）

### 资金面（最致命）

| # | 项 | 现状 | 风险 | 建议 | 状态 |
|---|---|---|---|---|---|
| B1 | 定价不是真实价格 | 已实现成本/售价分离：`model_pricing` 增加 `price_type`（cost/sell）与 `period`（peak/off_peak）；DeepSeek 官方价（2026-08-17 生效，含峰谷 + 缓存命中维度）已作为成本侧种子数据；售价 = 显式售价行 或 成本原价（无加价）；`PriceSnapshot` 记录 cost/sell/version/period；token 计费按 1K 换算；价格不完整 → 网关 422 `pricing_incomplete`（结算兜底按预留额计费并留证据） | 已消除 | 见 PROJECT_STATUS.md 二十六节 | ✅ 已实现（migration 000011 + pricer 双通道 + PAYG 门禁） |
| B2 | 账外注资 | 注册赠送余额仍直接 `INSERT wallets` 不写流水（`handler/console/auth.go`）；`ENABLE_FAKE_PAYMENT=false` 时赠送为 0，仅演示开关下触发 | 余额与流水不一致，对账失去根基 | 余额变更全部走 wallet repo 统一收口（TopUp/Transfer/专用 ledger 写入），禁止 handler 直改表 | 🟡 已定位，未修复 |
| B3 | 无 usage 的请求扣 0 | 非流式/流式上游缺失 usage 时回退请求体估算并显式标记 `source=estimated`（`chat.go`），不再静默免费 | 已消除 | 保留 estimated 标记与对账抽查，防止估算偏差累计 | ✅ 已实现 |
| B4 | 真实支付 | `ENABLE_FAKE_PAYMENT` 默认 `false`；易支付（epay）M0 已接入（下单/回调验签 + 幂等） | 官方支付渠道（支付宝/微信/Stripe）未接 | 继续扩展官方渠道适配器；支付回调验签 + 幂等已实现 | 🟡 易支付 M0 已接入，官方渠道待接 |
| B5 | 结算超额降级 | settle 失败（实际>预留）时 fallback commit 预留金，证据已标 `undercharged` | 少收路径仍在（已可见，未消除） | 预留按 max_tokens 上限计算，减少触发；对账按 undercharged 标记自动补收 | 🟡 证据已修，预留未改 |
| B6 | 历史证据缺口 | 旧 usage_log 的 `wallet_charged=0`（修复前未记录） | 历史对账不完整 | 迁移回填（类似 000010）或明确接受并记录 | ⬜ |

### 运维面

| # | 项 | 现状 | 风险 | 建议 | 状态 |
|---|---|---|---|---|---|
| B7 | 备份可恢复性 | 2026-08-19 清理工具初版备份为二进制乱码、不可恢复；`pg_dump` 有备份但从未演练恢复 | 数据丢失无法恢复 | 上线前做一次真实恢复演练（备份 → 清空 → 恢复 → 校验）；所有"先备份再删"工具必须验证备份可读 | 🟡 已记录教训，未演练 |
| B8 | 干净环境部署验证 | 只在本地开发环境跑通 | 新环境不可复现 | 从零跑一遍：新服务器 → compose → 迁移 → 健康检查 → 真实调用 → 对账；产出部署 runbook | ⬜ |
| B9 | 生产配置审计 | fail-fast 已拦弱密钥/COOKIE_SECURE，但 TLS 终止、密钥轮换、限流降级策略未演练 | 配置漂移/降级不可控 | 按 docs/DEPLOYMENT.md 逐项验收；admin 限流 Redis 挂了 fail-open 要有明确取舍 | 🟡 |

## 二、上线后尽快（决定能运营多久）

| # | 项 | 现状 | 建议 | 状态 |
|---|---|---|---|---|
| P1 | 月度账单 + 余额预警 | 已实现 | `GET /api/console/billing/statement`（GMT+8 自然月）+ `wallet/alert` 阈值 | ✅ |
| P2 | 上游成本对账 L2/L3 | 只有 L0/L1 | 对接各 provider 账单接口/导出，核对毛利；至少先做成本快照 vs 实际账单抽样 | ⬜ |
| P3 | 监控告警 | 有健康检查/日志，无指标与告警 | Prometheus 指标（请求量/延迟/错误率/计费差异）+ 错误上报 + 告警规则 | ⬜ |
| P4 | API Key 安全加固 | 明文回显接口存在（设计取舍）；密钥无轮换演练 | 回显加审计 + 权限收紧；JWT/ENCRYPTION_KEY 轮换演练 | ⬜ |
| P5 | 套餐/订阅 | 已实现 | 订阅套餐 CRUD + 购买/订单/自动续费/过期回收（000028-000032）；配额池已随 2026-08-25 重构移除 | ✅ |

## 三、按产品优先级（非上线必需）

| # | 项 | 说明 | 状态 |
|---|---|---|---|
| F1 | 剩余网关端点 | 已实现：/v1/responses、messages、count_tokens、images/edits、videos/generations（异步）、audio/transcriptions；剩余：视频下载 content/:index、Seedance 回调、/v1beta Gemini 原生端点（内部 GeminiAdapter 已可转换 chat） | 🟡 大部分已实现 |
| F2 | 聊天 UI | 体验增强（Playground 已有雏形） | ⬜ |
| F3 | OEM/白标 | 基础已实现：子账号客户管理（CRUD/封禁/角色）、同租户代充值、brand/runtime/settlement 配置、tenant_models 选品、租户级定价数据层（tenant_id 覆盖 + 平台回退）、PAYG 门禁；OEM 进阶（租户级定价管理入口、客户等级/AI 折扣、Owner 直接发额度、可见性裁剪、API Key 代管）已于 2026-08-25 明确不做 | ✅ |
| F4 | 阶梯折扣/多币种 | 收入策略与国际化，按需 | ⬜ |
| F5 | 质量债 | 覆盖率门禁未强制；-race 依赖 CI（本地无 gcc） | 🟡 |

---

## 四、建议路线图（Blocker 顺序）

1. **定价引擎**（B1）：售价/成本分离 + 固定价 + 缓存维度；deepseek 官方价作为第一个数据源（成本侧）——已实现且无加价（售价 = 成本）
2. **钱包账本收口**（B2）：余额变更全走流水；补余额预警（P1 前半）
3. **无 usage 计费策略**（B3）：估算并标记，禁止免费
4. **真实支付**（B4）+ 月度账单（P1 后半）
5. **恢复演练 + 干净环境部署验证**（B7/B8）+ 生产配置验收（B9）

> 完成 1-5 后即具备"对外收费上线"的最小条件；P2/P3（对账、监控）建议与 4 并行推进。

## 五、关联文档

- `docs/DEPLOYMENT.md` — 部署手册（环境变量基线、迁移、健康检查、备份、密钥轮换）
- `docs/PROJECT_STATUS.md` — 项目进度与变更记录（十九~二十一节为本轮审计与修复记录）
- `docs/DEEPTROLS_完整功能清单.md` — 功能清单（已按 2026-08-30 同步）

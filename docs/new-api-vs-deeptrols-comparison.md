# New API vs DeepTrols 对比分析

> 2026-08-04 | New API = `G:\workspace\demo\new-api-main` (AGPLv3, ~135K 行 Go)

## 定位

| | DeepTrols | New API |
|---|---|---|
| 基因 | 资金面优先（Reserve-Commit-Release） | 网关面优先（50+ Provider，格式转换） |
| 代码量 | ~15K 行 Go | ~135K 行 Go |

## 关键差异一览

| 维度 | DeepTrols | New API |
|---|---|---|
| Provider | LiteLLM 间接 | **50+ 内置适配器** |
| 格式转换 | ❌ | OpenAI ↔ Claude ↔ Gemini ↔ Responses |
| 高级自定义渠道 | ❌ | **Advanced Custom (type 58)** |
| 渠道亲和性 | ❌ | Redis 会话粘滞 |
| 定价引擎 | 9 维固定 Pricing 表 | **DSL 表达式引擎** (`pkg/billingexpr`) |
| 分层定价 | ❌ | `len <= 200000 ? tier("short") : tier("long")` |
| 订阅套餐 | ❌ | year/month/day/hour + 自定义 |
| 真实支付 | ❌ | Stripe / EPay(支付宝/微信) / Creem |
| OAuth 登录 | ❌ | GitHub/Discord/LinuxDO/OIDC/Telegram/WeChat |
| Passkey | ❌ | WebAuthn |
| RBAC | 二进制 admin/user | **Casbin v2 细粒度** |
| 前端 | React 18 + shadcn (刚迁移) | React 19 + shadcn + Semi Design (双 UI) |
| 数据库 | PostgreSQL 16 | SQLite/MySQL/PostgreSQL/**ClickHouse** |
| 缓存 | Redis | **Redis + 内存 LRU 混合** |
| 对账 | L0+L1 ✅ | ❌ |
| 多租户 | ✅ | ❌ |
| 分布式多节点 | ❌ | ✅ Casbin 策略同步 |
| 桌面端 | ❌ | Electron 系统托盘 |
| 排行榜 | ❌ | ✅ 模型/供应商排行 |

## DeepTrols 的护城河

| 能力 | New API |
|------|---------|
| 资金面（Reserve-Commit-Release + 乐观锁） | New API 有 PreConsume-Settle 但没有乐观锁和幂等 |
| 对账 L0+L1 | ❌ |
| 多租户隔离 | ❌ |
| 5 不变量 | ❌ |
| 响应缓存（零计费） | ❌ |

## 最值得借鉴的 4 项

| # | 功能 | 理由 |
|---|------|------|
| 1 | **Billing Expression Engine** | DSL 替换固定定价，十倍灵活性 |
| 2 | **Advanced Custom Channel** | 管理员自由配置上游，不需要写代码 |
| 3 | **格式转换管道** | OpenAI ↔ Claude ↔ Gemini，去 LiteLLM 依赖 |
| 4 | **渠道亲和性** | 会话粘滞提升用户体验 |

## 总结

```
DeepTrols → 最好的资金面对账引擎
New API   → 最好的 LLM 网关
CoAI      → 最好的聊天 UI + 一站式方案
```

三者互补。DeepTrols 在资金面和对账上无可替代，New API 在网关灵活性和商业化上领先。

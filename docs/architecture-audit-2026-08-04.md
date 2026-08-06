# DeepTrols vs 架构文档 — 全面审计报告

> 审计日期: 2026-08-04 | 对照文档: `AI聚合网关_完整文档.md`

## 一、总览

| 类别 | 数量 |
|------|------|
| ✅ 正确实现 | 20+ |
| 🟡 部分实现 | 6 |
| 🔴 严重缺失 | 4 |
| 🟠 OEM 缺失 | 4 |

---

## 二、🔴 严重缺口

| # | 问题 | 文档要求 | 现状 |
|---|------|---------|------|
| 1 | **HMAC 认证** | method+path+body SHA256 + ±300s + Redis nonce | 完全不存 在，只有 Bearer token |
| 2 | **流式错误伪装成功** | 错误不能伪装成正常成功 | `[DONE]` 无条件发送，status 始终 completed |
| 3 | **折扣引擎** | 用户等级/ OEM / 阶梯折扣 | 字段存在但永远为 0，无计算逻辑 |
| 4 | **Worker 选主** | Redis lease 选主 | 无任何实现，多实例重复执行 |

## 三、🟡 部分实现

| # | 问题 | 实际 | 文档要求 |
|---|------|------|---------|
| 5 | 健康检查 | 0/100 两级 | 渐进：<30 degraded, >70 recovering |
| 6 | 路由负载 | DB current_load | Redis INCR/DECR + Lua |
| 7 | `final_chunk` 标记 | 常量定义了但从未赋值 | 流式应标记 final_chunk |
| 8 | 租户 DB 故障 | fail-open 通过 | 未知 Host 不能落到平台 |
| 9 | 无钱包用户 | 跳过 reserve | 所有请求必须预算预留 |
| 10 | 价格快照 | 初始化为空 map | 须记录定价版本和数据来源 |

## 四、🟠 OEM 缺口

| # | 文档要求 | 现状 |
|---|---------|------|
| 11 | 租户范围内客户管理（封禁/调级/代充值） | 完全不存 在 |
| 12 | 租户创建时初始化 brand_config / runtime_config | 字段为 nil |
| 13 | OEM 自助模型定价管理 | 表存在但无 UI |
| 14 | 代充值（同租户钱包转账） | TransferIn/Out 常量存在但无实现 |

## 五、✅ 已正确实现

### 5 不变量

1. request_id + compound identity ✅
2. Reserve → Execute → Commit/Release ✅
3. channel_id + instance_id + route_policy_id → 证据链 ✅
4. upstream / estimated / cached 标记 ✅
5. 流式错误处理 ⚠️（部分）

### 核心计费链路

Reserve-Commit-Release + 乐观锁 + 幂等 + 9维定价 + decimal + 三表事务 + Outbox + Committer(100%测试) + 流式闭环 + 配额Check→429 ✅

### 控制面

API Key 6边界 + JWT httpOnly + TOTP + 租户5状态机 + fail-closed + 字段保护 + 模型CRUD + Provider Sync ✅

### 前端

16 Console页面 + shadcn/ui 21页 + SectionPageLayout + StateViews + 用户CRUD + 配额Create/Allocate + 钱包(订单号+状态+支付方式+￥) ✅

### 新增（超出文档要求）

响应缓存(SHA256→Redis零计费) + Docker 5容器一键部署 + Air+Vite HMR热重载 ✅

## 六、实施优先级

**本周（风险修复）**: 流式错误不伪装(1d) + 租户DB故障fallback(0.5d) + 无钱包用户拦截(0.5d)

**本月（功能补齐）**: HMAC认证(2-3d) + Worker选主(1d) + 健康检查渐进评分(1d) + 路由Redis负载(1d) + final_chunk标记(0.5d) + 价格快照填充(0.5d)

**商业化前**: 折扣引擎(1-2w) + OEM客户管理(1-2w) + 支付网关(3-5d)

**MVP外**: 网关扩展端点 + L2/L3对账 + 多币种

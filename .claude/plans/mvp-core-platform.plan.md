# Plan: AI Token 聚合平台 MVP

**Source PRD**: `docs/AI聚合平台_产品需求文档_PRD.md`
**架构文档**: `docs/AI聚合网关_完整文档.md`
**Last Updated**: 2026-07-28

---

## 产品架构：用户端 vs 管理端

```
┌──────────────────────────────────────────────────────────────┐
│              用户端 (Developer Portal)                        │
│  面向: 开发者、调用方                                          │
│  功能: API Key │ 模型广场 │ Playground │ 用量 │ 钱包 │ 文档   │
│  鉴权: JWT (注册/登录)                                        │
├──────────────────────────────────────────────────────────────┤
│              API 网关 (OpenAI-compatible)                     │
│  面向: 机器调用                                                │
│  接口: /v1/chat/completions │ /v1/models │ /v1/embeddings   │
│  鉴权: API Key (Bearer token)                                │
├──────────────────────────────────────────────────────────────┤
│               管理端 (Admin Console)                          │
│  面向: 平台运营人员                                            │
│  功能: 模型管理 │ Provider凭证 │ Channel │ 路由策略 │ 租户     │
│        配额 │ 对账 │ 用户管理                                  │
│  鉴权: JWT + Admin 角色                                       │
└──────────────────────────────────────────────────────────────┘
```

## 架构决策

1. **URL 路径分离**: 用户端 `/console/*`，管理端 `/admin/*`
2. **角色权限**: JWT claims 含 `role` 字段（`user` / `admin`）
3. **前端分离**: Layout 根据角色显示不同导航
4. **后端分离**: Admin middleware 校验 admin 角色

---

## 一、用户端 (Developer Portal)

> URL: `/console/*`
> 页面: 工作台 / API密钥 / 模型广场 / Playground / 调用日志 / 用量 / 钱包 / 安全 / 文档

### 1.1 接入

| 功能 | 状态 |
|---|---|
| 注册（email + password → 自动创建钱包） | ✅ |
| 登录（bcrypt + JWT） | ✅ |
| Quickstart 引导 | ✅ 文档页已含 |

### 1.2 API Key 管理

| 功能 | 状态 |
|---|---|
| 创建/删除 Key | ✅ |
| 6 边界配置（模型/IP/限额/策略/状态/用量） | ✅ 35 测试 |
| Plaintext 一次显示 + 警告 | ✅ |

### 1.3 模型 & Playground

| 功能 | 状态 |
|---|---|
| 模型目录（含定价） | 🟡 有列表，缺详情页 |
| Playground（选 Key + 模型 + 对话） | ✅ 15 测试 |
| 流式输出 | ❌ |

### 1.4 用量 & 钱包

| 功能 | 状态 |
|---|---|
| 实时余额 + 冻结 | ✅ |
| 交易流水 | ✅ |
| 调用日志列表 | 🟡 缺 charge line 展开 |
| 用量趋势图 | 🟡 硬编码数据 |
| 充值 | ❌ |
| 月度账单 | ❌ |
| 余额预警 | ❌ |

### 1.5 安全 & 文档

| 功能 | 状态 |
|---|---|
| MFA/TOTP | ❌ |
| 开发文档（Quickstart/API/模型/计费） | ✅ 30 测试 |

---

## 二、API 网关

| 接口 | 状态 |
|---|---|
| `POST /v1/chat/completions` | ✅ |
| `GET /v1/models`（读 DB + Key 过滤） | ✅ 7 测试 |
| 流式 SSE | ❌ |
| embeddings/images/audio | ❌ |
| 限流（RPM/TPM） | ❌ |

---

## 三、管理端 (Admin Console)

> URL: `/admin/*`
> 权限: JWT role=admin
> 页面独立于用户端

### 3.1 模型管理

| 功能 | 状态 |
|---|---|
| 模型 CRUD + 多维定价 | ✅ |
| 前端管理页面 | ✅ |

### 3.2 Provider 凭证管理

| 功能 | 状态 |
|---|---|
| 录入上游 API Key（OpenAI/Anthropic 等） | ❌ |
| 加密存储 | ❌ |

### 3.3 Channel & 转发实例

| 功能 | 状态 |
|---|---|
| 转发实例管理（地址/凭证/并发） | ❌ |
| Channel 管理（绑定模型+实例、权重/健康） | ❌ |
| 路由策略（fallback 配置） | ❌ |

### 3.4 租户/OEM

| 功能 | 状态 |
|---|---|
| 租户生命周期 | ❌ |
| 品牌/模型/定价配置 | ❌ |

### 3.5 财务 & 对账

| 功能 | 状态 |
|---|---|
| 配额管理 | ❌ |
| 人工充值 | ❌ |
| 对账 L0-L3 | ❌ |

---

## 四、待做：用户端/管理端分离

> 当前状态：所有页面混在同一个 Layout，无角色区分
> 需要做的：

| 步骤 | 内容 |
|---|---|
| 1 | JWT 加 role 字段（admin/user），登录时注入 |
| 2 | 后端 `/admin/*` 路由 + AdminAuth middleware |
| 3 | 前端 Layout 拆分为 UserLayout + AdminLayout |
| 4 | 用户端页面迁到 `/console/*` |
| 5 | 管理端页面迁到 `/admin/*` |
| 6 | 注册用户默认 role=user，bootstrap admin=admin |

## 五、实施顺序

## 工作流程优化

| 层 | 执行方式 | 原因 |
|---|---|---|
| 后端 Go + DB | tdd-guide agent | 需要 DB 测试隔离 |
| 前端 React | 主循环内联 | vitest 快，上下文已加载 |
| 代码审查 | code-reviewer agent | 需要独立视角 |
| 安全审查 | security-reviewer agent | 同上 |

1. **分离用户端/管理端** — JWT role + 路由 + Layout 拆分
2. **Provider 凭证管理**
3. **Channel + 转发实例管理**
4. **用户端完善** — 模型详情/charge line/用量真实 API/充值
5. **对账系统**
6. **租户/OEM + 多模态 + 流式**

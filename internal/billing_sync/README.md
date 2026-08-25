# billing_sync（账单同步，移植自 TokenHub）

> 源码：TokenHub（https://github.com/astaxie/TokenHub），Apache-2.0，直接拷贝，见 `NOTICE`。

Provider 无关的上游账单同步器：把 OneAPI / NewAPI / 阿里云的账单拉取并归一化为统一的
`Record`，供对账与用量分析使用。对应蓝图 Step 1a，用于补齐"对账 L3（L1 ↔ 上游账单）"。

## 结构

```
types.go       Connector / Record / SyncRun / Adapter / Store / ManagementStore / 错误类型
service.go     同步调度服务（RunDue / 定时器 / 审计）
validation.go  连接器配置校验
adapters/      OneAPI / NewAPI / 阿里云 账单适配器（HTTP 拉取 → FetchPage）
```

## 接口（端口）

- `Adapter`：上游账单源唯一能力（Fetch）。
- `Store`：同步服务所需的持久化端口。
- `ManagementStore`：连接器管理所需的持久化端口。

## 待接入（本仓库部分）

- `internal/billing_sync/persistence`：pgx 实现 `Store` / `ManagementStore`（TokenHub 原
  实现基于 GORM，不搬运）。
- `migrations`：billing_connectors / billing_sync_runs / billing_records / raw_snapshots。
- Worker 定时同步（复用 `internal/pkg/lease` 选主）。
- Console/Admin API + 对账 L3 接线。

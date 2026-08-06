# 定价加价功能 — 完成

> 日期: 2026-08-03
> 依据: 成本核算发现所有模型 profit=0（定价=成本无加价），运营需盈利
> 结果: **加价 2.0 已应用（74 条定价），后端 28 包 + 前端 182 测试全绿**

---

## 背景

成本核算上线后确认：所有模型 `unit_price == upstream_cost`（0.001），利润为 0。自营转售需加价才能盈利。

## 实现

### 后端 `internal/handler/console/pricing.go`
- `HandleSetMarkup`：`POST /api/admin/pricing/markup`，body `{markup_rate: 2.0}`
  - 校验 rate ≥ 1
  - 批量更新：`unit_price = upstream_cost * rate`（仅 `upstream_cost > 0` 的行）
  - 返回 `{rows_updated}`
- `HandleGetMarkup`：`GET /api/admin/pricing`，返回定价/模型数概览

### 前端 `web/src/pages/Costs.tsx`
- 成本页顶部加「设置加价率」控件：输入倍率 → 应用 → 显示更新行数 → 刷新成本

### 路由
- `cmd/api/main.go`：`POST /pricing/markup` + `GET /pricing`

---

## 验证

- ✅ 后端 28 包全绿
- ✅ 前端 182/182 测试 + TSC + build 通过
- ✅ 冒烟：应用加价 2.0 → `rows_updated: 74` → DB 确认 `markup = 2.0`（0.002 / 0.001）

---

## 运营含义

- 加价 2.0 = 售价为成本 2 倍 → **利润率 50%**
- 成本报表的 `final_cost` 是**历史调用快照**（加价前发生），新调用按新价计费
- 可随时调整加价率（如 1.5 / 3.0）

---

## 遗留（记录）

- 阶梯折扣（量大打折）未实现
- 加价是全局统一倍率，未支持按模型差异化加价

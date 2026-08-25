# guardrails（出站内容策略引擎，移植自 TokenHub）

> 源码：TokenHub（https://github.com/astaxie/TokenHub），Apache-2.0，直接拷贝，见 `NOTICE`。

网关出站内容策略：在请求发往上游前对提示词/输出片段做确定性检测（关键词、正则、敏感数据），
并支持基于模型（Qwen3Guard）的检测器；动作 allow / audit / mask / block，带 ReDoS 工作量
预算与判定合并（严格优先、block 短路）。

## 结构

```
engine.go       Evaluate：策略匹配 → 确定性匹配器（工作预算）→ 模型检测 → 决策合并
policy.go       策略/检测项/绑定模型 + NormalizePolicy 校验与规范化
qwen_detector.go Qwen3Guard 模型检测器（HTTP，可选）
persistence/    pgx 仓储：加载策略（含检测项与绑定）
```

## 接线状态

- 引擎与表结构已就位（`migrations/000016_guardrails`）。
- 网关接线（chat 路由前评估、block 返回 400、落审计）与 Admin 策略编辑器属于
  蓝图 Phase 1（Step 4），未在本批实现。

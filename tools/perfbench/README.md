# perfbench（网关压测工具，移植自 TokenHub）

> 源码：TokenHub（https://github.com/astaxie/TokenHub），Apache-2.0，直接拷贝，见 `NOTICE`。

对 OpenAI 兼容网关做确定性压测：并发/固定速率两种负载模式，流式与非流式，输出
延迟/吞吐/上游开销估计的 JSON 或 Markdown 报告，支持预算回归检查。

## 用法

```bash
# 起一个确定性 mock 上游
go run ./cmd/perfbench mocker -listen 127.0.0.1:18081

# 对网关压测
go run ./cmd/perfbench run -base-url http://127.0.0.1:18081 \
  -api-key sk-test -model deepseek-chat -duration 30s -concurrency 20
```

## 结构

```
runner.go    负载执行（并发/固定速率、流式、超时、去重）
mocker.go    确定性 OpenAI 兼容上游（含故障注入）
analyze.go   观测汇总（延迟/吞吐/上游开销）
budget.go    预算回归检查
report.go    JSON / Markdown 输出
go_benchmark.go Go 基准解析与预算检查
```

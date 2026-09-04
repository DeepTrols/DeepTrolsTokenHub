# TH-P1-AL-01 执行日志 — Alipay Config And Startup Validation

- 日期：2026-09-04（Sprint 1 Batch 7）
- 状态：DONE
- 业务提交：见 §3
- 依赖：TH-P1-03（DONE）

## 0. 实现方式

支付宝渠道的配置与启动校验层，按 TDD（RED→GREEN）：先写
`alipay_config_test.go`（9 用例）+ 指标断言令两包编译失败，再实现配置
类型、校验、Info/下单接线与就绪度指标。CreateOrder / notify / query
客户端保持 Out of Scope（TH-P1-AL-02+ 未动）。

设计决策：

1. **配置集分离**：生产（`alipay_app_id` / `alipay_private_key` /
   `alipay_gateway_url`）与沙箱（`alipay_sandbox_*`）两套独立取值，
   `alipay_sandbox` 布尔选择生效集；网关地址缺省回落官方端点（生产
   `https://openapi.alipay.com/gateway.do`、沙箱
   `https://openapi-sandbox.dl.alipaydev.com/gateway.do`），覆盖值必须
   为绝对 https 且有 host。
2. **构造性脱敏（fail-fast + AC-03）**：`AlipayConfig.Validate()` 的
   错误**只含配置键名**（缺哪些键、哪个键不是合法 https URL），取值
   （私钥、APPID、URL 原文）在构造上不可能进入错误文本；单测以独特
   私钥串断言错误与**日志输出**（捕获 `log` 写入）均不含密钥体。
3. **支付信息检查（AC-01/AC-02）**：`PaymentInfo` 新增
   `channel_error`（脱敏诊断，omitempty）。`payment_channel=alipay`
   且配置不全 → `channel_error` 点名缺失键、不展示任何支付方式；
   配置完整 → 展示支付宝方式（仍受 `payment_enabled` + 合规闸门约束）；
   epay 渠道行为完全不变（回归断言 `channel_error` 为空 + 2 方式）。
4. **下单失败关闭**：`CreateOrder` / `CreateSubscriptionOrder` 在任何
   网关调用**之前**执行 `validateChannelConfig`——配置不全 →
   `ErrChannelConfigInvalid`（新哨兵错误，包裹脱敏诊断）、零订单行；
   配置完整时仍由工厂在 TH-P1-AL-02 落地前返回 `ErrChannelNotReady`
   （两段皆失败关闭，次序由测试固化）。
5. **可观测性**：新增 `payment_channel_config_ready{channel}` gauge
   （1=就绪/0=未就绪），由支付信息检查按生效渠道置值；标签经
   `SanitizePaymentRoute` 白名单钳制（epay/alipay/wechatpay→其余
   other），配置取值绝不进入标签。

### 配置键清单（文档要求，已同步 docs/DEPLOYMENT.md §1.1）

`payment_channel`、`alipay_sandbox`、`alipay_app_id`、
`alipay_private_key`、`alipay_gateway_url`、`alipay_sandbox_app_id`、
`alipay_sandbox_private_key`、`alipay_sandbox_gateway_url`。
回滚：将 `payment_channel` 切离 `alipay`；校验失败时创建与结算保持
失败关闭，绝不降级到错误渠道。

## 1. AC 验证

| AC | 要求 | 验证测试 | 结果 |
|---|---|---|---|
| AC-01 | `payment_channel=alipay` 且缺 app id → 支付信息检查报配置错误 | TestInfoAlipayChannelMissingAppIDReportsConfigError（`channel_error` 点名 `alipay_app_id`、无方式、不泄露已配置的私钥）+ TestCreateOrderAlipayFailFastOnMissingConfig（下单 `ErrChannelConfigInvalid`、0 行） | PASS |
| AC-02 | 必填字段齐全 → 支付信息报告支付宝方式可用 | TestInfoAlipayChannelReadyReportsMethod（`channel_error` 空、方式=alipay）+ TestInfoAlipayReadyButPaymentDisabled（全局闸门仍生效） | PASS |
| AC-03 | 校验失败时日志不含私钥/证书体 | TestAlipayConfigValidateRedactsSecrets（错误层）+ TestCreateOrderAlipayFailFastOnMissingConfig（捕获日志断言不含密钥串） | PASS |

Test Requirements 逐项：

- Unit（配置校验表）：TestAlipayConfigValidateTable（10 行表：生产/沙箱
  完整、缺 APPID/私钥/双缺、URL 非 URL/非 https、沙箱选值隔离等）
  + TestConfigLoadsAlipaySettingsFromSettingsStore（settings 键映射 +
  沙箱选择端到端）PASS。
- Integration（Alipay 渠道服务信息）：TestInfoAlipay* 三用例经
  `Service.Info` 全路径（配置装载→校验→Info 组装）PASS。
- Regression（epay 不变）：TestInfoEpayChannelErrorEmpty +
  TestInfoListsPayMethods + TestFactorySelectsEpay +
  TestCreateOrderRecordsEpayChannel 保持绿。
- Failure Injection（缺密钥 + 畸形 URL）：校验表缺键/畸形 URL 行 +
  TestInfoAlipayMalformedURLReportsConfigError（`channel_error` 点名
  `alipay_gateway_url`）PASS。
- 工厂守卫回归：TestFactoryAlipayNotReadyIsConfigError /
  TestCreateOrderAlipayValidConfigStillNotReady（有效配置下仍
  `ErrChannelNotReady`，AL-02 边界未越权）/ 未知渠道
  `ErrInvalidChannel` PASS。

## 2. 证据

- RED：`go vet` 显示 metrics 与 payment 包编译失败
  （`SetPaymentChannelConfigReady` / `AlipayConfig` 不存在）。
- GREEN：payment 包全部用例（9 新增 + 工厂/回归全量）与 metrics 包
  （新家族进入 scrape 清单）PASS。
- `go vet ./...` exit 0；`go build ./...` exit 0；`gofmt -l` clean。
- 独立全量回归（TH-P05-13 门控退出码契约）：
  `GATE_LOG_FILE=/tmp/p1-gate/p1-al-01.log scripts/gate_command.sh go test ./... -count=1`
  → 49 包 `ok`、0 `FAIL`/panic、日志完整（3442 字节），go test 退出码 0。

## 3. 提交

- 业务提交 `dbb30ca`（Sprint 1 Batch 7，与 TH-P1-05 同批；15 files,
  +1229/−18）。
- `web/**` 与既有未跟踪文档未纳入。

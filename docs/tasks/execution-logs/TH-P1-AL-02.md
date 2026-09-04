# TH-P1-AL-02 执行日志 — Alipay CreateOrder Client

- 日期：2026-09-04（Sprint 1 Batch 8）
- 状态：DONE
- 业务提交：见 §3
- 依赖：TH-P1-AL-01（DONE）

## 0. 实现方式

支付宝创建订单客户端，按 TDD（RED→GREEN）：先写 `alipay_test.go`
（10 用例：金额格式化、私钥解析、签名可验证性、参数映射、沙箱下单、
提供方错误、超时、回调/查单失败关闭）令包编译失败，再实现
`internal/service/payment/alipay.go` 与工厂接线。

设计决策：

1. **网关与签名契约**：`AlipayGateway{AppID, PrivateKey *rsa.PrivateKey,
   GatewayURL, NotifyURL, HTTPClient}` 调 `alipay.trade.precreate`。
   RSA2 签名 = 按键名排序的 `k=v` 以 `&` 连接（排除 `sign` 参数与空值）
   → SHA256 摘要 → `rsa.SignPKCS1v15` → base64；biz_content 为
   `{"out_trade_no","total_amount"(decimal 两位),"subject"}`；公共参数
   含 app_id/method/format=JSON/charset=utf-8/sign_type=RSA2/timestamp/
   version=1.0/notify_url（可选）/biz_content。表单以
   `application/x-www-form-urlencoded;charset=utf-8` POST。
2. **响应解析与错误映射（fail closed）**：信封键
   `alipay_trade_precreate_response`；成功 = `code=="10000"` 且 qr_code
   非空。失败族：非 200（含 http_status）、invalid_json、
   missing/invalid_response_node、code≠10000（携带消毒后的
   code/sub_code）、missing_qr_code。新增哨兵 `ErrAlipayProvider` /
   `ErrAlipayTimeout`：均不产生订单行、不产生钱包影响；提供方身份按
   Risk 条目保留——`sanitizeAlipayCode` 把 code/sub_code 钳制在
   `[A-Za-z0-9_.-]` ≤64 字符后写入错误文本，标签空间则经
   `SanitizeAlipayOutcome` 白名单钳制。
3. **私钥与配置路径**：`parseAlipayPrivateKey` 接受 PKCS8 PEM、
   PKCS1 PEM 或裸 base64，错误信息脱敏（绝不回显密钥内容）。工厂
   `newGatewayForChannel` 的 alipay 分支：`Validate()` →
   `effective()`（沙箱/生产配置集选择）→ 解析私钥 → 构造网关
   （NotifyURL = callback_base + `/api/payment/notify/alipay`）。
   通知路由构造绕过 CreateOrder 的 fail-fast 门，故工厂自身必须复检
   配置。错误类语义迁移：alipay 配置缺失/不可解析从 AL-01 时代的
   `ErrChannelNotReady` 迁为 `ErrChannelConfigInvalid`；wechatpay 保留
   `ErrChannelNotReady`；三个既有测试随之手术式更新并留注释。
4. **可观测性（Observability Requirement）**：新增
   `payment_alipay_create_total{outcome}` 与
   `payment_alipay_create_duration_seconds{outcome}`（durationBuckets），
   outcome ∈ {success, provider_error, timeout, error}；
   `CreateOrder` 以 defer 记录，错误按类归并。指标家族在
   `TestHandler_Scrapeable` 实体化并经 `TestGatheredFamilies` 敌意
   输入泄漏检查。
5. **测试策略（Integration: mocked Alipay client）**：不 mock 签名/
   解析逻辑——用 `httptest.NewTLSServer`（https 满足 AL-01 校验）+
   `srv.Client()` 仅替换传输层；真实工厂经 `s.newGateway` 包装注入，
   模拟网关在服务端用真实公钥做 RSA2 验签（非法签名返回 400，
   `atomic` 计数断言验签确实执行）。settings 以裸 base64 承载私钥
   （PEM 换行在 JSON 字符串里非法），PEM 形态由解析器单测覆盖。
   AC-01 需在 settings 设 `min_topup: "0.01"`（默认 1 会拒绝）。
6. **范围外占位（Out of Scope）**：`VerifyNotify` →
   `ErrChannelNotReady`（回调验签属 TH-P1-AL-03）；`QueryOrder` →
   `ErrQueryUnsupported`（对账查单属 TH-P1-AL-05）。两者均失败关闭。
7. **文档（Documentation Requirement）**：沙箱下单探针写入
   `docs/DEPLOYMENT.md` §1.1（回滚段之后）：探针步骤、fail-closed
   行为、自动化测试的同一契约。

## 1. AC 验证

| AC | 要求 | 验证测试 | 结果 |
|---|---|---|---|
| AC-01 | 有效沙箱配置 + 0.01 金额 → 返回非空支付 URL 与本地订单号 | TestCreateOrderAlipaySandboxReturnsPayURL（pay_url 持久化、订单 pending、服务端验签通过） | PASS |
| AC-02 | 支付宝返回提供方错误 → 提供方错误类、无已支付订单状态 | TestCreateOrderAlipayProviderErrorFailsClosed（40004/ACQ.INVALID_PARAMETER 消毒保留、0 订单行、0 钱包入账） | PASS |
| AC-03 | 上下文超时 → 超时错误、无钱包交易 | TestCreateOrderAlipayTimeoutFailsClosed（50ms ctx vs 500ms handler，ErrAlipayTimeout，0 钱包入账） | PASS |

Test Requirements 逐项：

- Unit（请求映射与金额格式化）：TestFormatAlipayAmount（0.01/10/1.5/
  1.234→1.23/1.239→1.24/100 的 decimal 两位钳制）+
  TestBuildAlipayParamsMapsRequest（固定时钟下公共参数与 biz_content
  映射）+ TestAlipaySignParamsVerifiable（真实公钥验签，空值与 sign
  参数排除）+ TestParseAlipayPrivateKeyShapes（PKCS8 PEM/裸 base64/
  PKCS1 PEM/垃圾输入脱敏/空值）PASS。
- Integration（mocked Alipay client）：TestCreateOrderAlipaySandbox-
  ReturnsPayURL + 提供方错误 + 超时三用例走真实工厂、真实签名、TLS
  模拟网关（仅换传输层）PASS。
- Regression（创建后本地订单保持 pending）：AC-01 用例断言订单状态
  pending、渠道字段正确、`fakeWallets.topupCount==0`；epay 回归面
  未触碰；既有工厂/配置测试手术式更新后保持绿。
- Failure Injection（提供方错误与超时）：40004/ACQ.INVALID_PARAMETER
  与 500ms 超时注入均失败关闭；另有 TestAlipayGatewayNotifyAndQuery-
  FailClosed 与 TestNotifyAlipayRouteFailsClosedUntilCallbackTask
  （回调/查单在 AL-03/AL-05 落地前失败关闭）。

## 2. 证据

- RED：`go vet` 显示 service/payment 包编译失败（`AlipayGateway`、
  `parseAlipayPrivateKey`、`ErrAlipayProvider`/`ErrAlipayTimeout`、
  `RecordAlipayCreateOrder` 均不存在）。
- GREEN：alipay_test.go 10 用例 + 手术更新的工厂/配置/通知路由测试
  全部通过（`go test ./internal/service/payment/... -run Alipay -v`
  与 Factory/Config/Notify 相关 23 例逐一 PASS）；metrics 包全绿。
- `go vet ./...` exit 0；`go build ./...` exit 0；`gofmt -l` clean。
- 独立全量回归（TH-P05-13 门控退出码契约）：
  `GATE_LOG_FILE=/tmp/p1-gate/p1-al-02.log scripts/gate_command.sh go test ./... -count=1`
  → 50 包 `ok`、0 `FAIL`/panic、日志完整（3507 字节，与 CW-01 门控
  MD5 不同，确认为独立运行），go test 退出码 0。

## 3. 提交

- 业务提交：`0d2ace3`（Batch 8 收口提交，与 TH-P1-CW-01 同批，
  16 files, +1517/−28），`web/**` 与既有未跟踪文档不纳入。

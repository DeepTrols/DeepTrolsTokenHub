# P1 Payment

## Code Audit Before Task Creation

Existing:

- `internal/service/payment/types.go` 已定义 `Gateway`，当前只有 `CreateOrder` 与 `VerifyNotify`。
- `internal/service/payment/epay.go` 已实现易支付下单与回调验签。
- `internal/service/payment/service.go` 已覆盖充值订单、订阅订单、回调入账、管理员补单。
- `payment_orders` 已有 `pending/paid/closed/refunded` 状态机和 `expires_at`。
- 支付入账通过 `wallet_transactions.idempotency_key=order_no` 保持幂等。
- `/api/payment/notify/epay` 为公开回调路由，安全依赖签名验证。

Partial:

- `payment_channel` 已读取，但 gateway factory 仍硬编码 epay。
- 手工补单存在，但 provider 主动查单和丢回调补偿不存在。
- 官方支付宝/微信没有直接适配器、启动校验、沙箱测试或生产小额验证。
- 支付对账没有接 provider 官方账单。

Missing:

- `QueryOrder` 契约与 provider 状态归一。
- 支付通道 factory、provider metadata、按订单通道回调。
- Alipay 7 个可验收子任务。
- WeChat 7 个可验收子任务。
- Compensation Worker 9 个可验收子任务。
- Provider 支付账单对账与 epay 退场任务。

Risks:

- 若未先完成 P0.5 gate，真实支付会带着 B5/B7/B8 风险前进。
- Provider adapter 合成一个大任务会导致创建订单完成但查单/回调仍不可见。
- 通道切换若不按订单 channel 回调，历史 pending 订单会被错误 gateway 处理。
- 支付日志可能泄露证书、私钥、签名、支付 URL 或交易号。

---

## Sprint Tasks

### TH-P1-01

Task ID: TH-P1-01
Title: QueryOrder Result Contract
Phase: P1
Epic: Payment
Type: Backend / Contract
Priority: P1
Dependencies: TH-P05-09
Status: DONE (Batch 5 — provider-neutral QueryOrder contract, see execution-logs/TH-P1-01.md)

Objective:

Define provider-neutral active order inquiry result fields and state values.

Current State:

`Gateway` has no `QueryOrder`; provider paid/unpaid/closed states have no common shape.

Scope:

Add result type, state enum, retryable flag, and amount fields.

Out of Scope:

Provider clients and worker scheduling.

Implementation Notes:

Keep the contract usable by Alipay, WeChat, and temporary epay checks.

Acceptance Criteria:

- AC-01: Given provider state `paid`, contract can carry local order number, provider trade number, amount, method, paid time, and retryable=false.
- AC-02: Given provider timeout, contract can carry retryable=true without paid fields.
- AC-03: Given unknown provider state, contract maps to `unknown` and no local transition intent is implied.

Test Requirements:

- Unit: state enum and validation.
- Integration: fake gateway compile-time interface check.
- Regression: current epay tests compile after interface adaptation.
- Failure Injection: unknown state and malformed amount.

Observability Requirements:

No metrics required; later tasks emit query outcome.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration. Rollback removes the interface extension before provider tasks land.

Documentation Requirements:

Document provider-neutral states.

Risks:

Ambiguous state names can cause false settlement. Keep enum closed.

Definition of Done:

Global DoD applies.

### TH-P1-02

Task ID: TH-P1-02
Title: QueryOrder Settlement Intent Service
Phase: P1
Epic: Payment
Type: Backend / Service
Priority: P1
Dependencies: TH-P1-01
Status: DONE (Batch 6 — settlement intent table, see execution-logs/TH-P1-02.md)

Objective:

Convert `QueryOrder` results into local settlement intents without mutating wallet balance directly.

Current State:

Callback settlement exists, but active query flow does not exist.

Scope:

Add service logic for `paid`, `not_paid`, `closed`, `timeout`, `amount_mismatch`, and local already-paid cases.

Out of Scope:

Worker scan loop, provider clients.

Implementation Notes:

The service returns intent; specific caller decides whether to execute settlement.

Acceptance Criteria:

- AC-01: Given query result `paid` with matching amount, service returns `mark_paid`.
- AC-02: Given matching order is already `paid`, service returns `already_settled` and creates no wallet call.
- AC-03: Given query result amount differs from local amount, service returns `amount_mismatch` and leaves local order unchanged.
- AC-04: Given timeout, service returns retryable result and leaves local order unchanged.

Test Requirements:

- Unit: intent table tests.
- Integration: fake order repo and fake wallet call counter.
- Regression: notify settlement behavior remains unchanged.
- Failure Injection: missing order and amount mismatch.

Observability Requirements:

Emit query intent outcome by channel and state in later worker/provider tasks.

Audit Requirements:

No audit row unless a manual admin action invokes it.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document intent table.

Risks:

Settlement intent must not bypass callback amount checks.

Definition of Done:

Global DoD applies.

### TH-P1-03

Task ID: TH-P1-03
Title: Payment Channel Factory
Phase: P1
Epic: Payment
Type: Backend / Integration
Priority: P1
Dependencies: TH-P1-01
Status: DONE (Batch 5 — channel factory fail-closed selection, see execution-logs/TH-P1-03.md)

Objective:

Make configured payment channel select the correct gateway implementation.

Current State:

`payment_channel` is read, but the service always creates `EpayGateway`.

Scope:

Add factory selection for `epay`, `alipay`, and `wechatpay` with disabled-channel errors.

Out of Scope:

Provider-specific client implementation.

Implementation Notes:

Use fake placeholders until provider tasks fill concrete implementations.

Acceptance Criteria:

- AC-01: Given `payment_channel=epay`, factory returns epay gateway.
- AC-02: Given `payment_channel=alipay` before Alipay config is complete, factory returns a channel configuration error.
- AC-03: Given unknown channel value, order creation returns invalid channel error and creates no `payment_orders` row.

Test Requirements:

- Unit: channel factory table tests.
- Integration: service order creation with fake gateway.
- Regression: epay order creation remains green.
- Failure Injection: unknown channel.

Observability Requirements:

Log selected channel and config error class, never credentials.

Audit Requirements:

Admin settings changes remain covered by admin audit middleware.

Migration / Rollback Requirements:

No migration. Rollback returns factory default to epay.

Documentation Requirements:

Document channel values and temporary epay fallback.

Risks:

Wrong default can route paid traffic to the wrong gateway.

Definition of Done:

Global DoD applies.

### TH-P1-04

Task ID: TH-P1-04
Title: Callback Route Channel Resolver
Phase: P1
Epic: Payment
Type: Backend / Routing
Priority: P1
Dependencies: TH-P1-03
Status: DONE (Batch 6 — per-channel callback resolver, see execution-logs/TH-P1-04.md)

Objective:

Resolve callback gateway by callback route and local order channel, not only current global config.

Current State:

Only `/api/payment/notify/epay` exists.

Scope:

Add route resolver foundation for `/alipay` and `/wechatpay`, and reject route/order channel mismatch.

Out of Scope:

Provider signature verification implementation.

Implementation Notes:

Historical pending orders must settle through their original channel during cutover.

Acceptance Criteria:

- AC-01: Given an epay order, `/notify/epay` can reach epay verification.
- AC-02: Given an epay order payload posts to `/notify/alipay`, resolver rejects before wallet settlement.
- AC-03: Given route has no matching provider, callback returns provider failure text and local order remains pending.

Test Requirements:

- Unit: route-to-channel resolver.
- Integration: callback route mismatch case.
- Regression: existing epay notify test remains green.
- Failure Injection: missing order number.

Observability Requirements:

Metric callback route mismatch by route and order channel.

Audit Requirements:

No audit row for automatic callback.

Migration / Rollback Requirements:

No migration. Rollback removes new route registrations.

Documentation Requirements:

Document callback URL per channel.

Risks:

Mismatch acceptance can mark wrong orders paid.

Definition of Done:

Global DoD applies.

### TH-P1-05

Task ID: TH-P1-05
Title: Payment Order Provider Metadata
Phase: P1
Epic: Payment
Type: Backend / Data Contract
Priority: P1
Dependencies: TH-P1-03, TH-P05-10
Status: DONE (Batch 7 — migration 000037 nullable provider metadata, see execution-logs/TH-P1-05.md)

Objective:

Ensure local payment orders carry enough provider metadata for channel-specific callbacks, query, and reconciliation.

Current State:

`payment_orders` has channel, method, trade number, pay URL, notify raw, and timestamps, but provider query/reconciliation metadata has not been reviewed.

Scope:

Audit existing columns, add only missing nullable metadata needed for provider query/retry/review, and add tests.

Out of Scope:

Provider implementation and worker loop.

Implementation Notes:

Prefer nullable additive migration if metadata is missing.

Acceptance Criteria:

- AC-01: Given a newly created order, row includes channel, method, order number, amount, expiry, and pay URL.
- AC-02: Given provider query needs retry metadata and no column exists, migration adds nullable retry fields with down migration.
- AC-03: Given old rows lack new metadata, list and query flows return valid JSON and no panic.

Test Requirements:

- Unit: metadata DTO mapping.
- Integration: migration up/down if added.
- Regression: old payment order rows remain readable.
- Failure Injection: null metadata.

Observability Requirements:

No sensitive metadata in logs.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

Nullable additive migration only; down migration documented.

Documentation Requirements:

Document provider metadata fields.

Risks:

Adding non-null fields can break existing rows. Use nullable/backfill-safe design.

Definition of Done:

Global DoD applies.

## Alipay

### TH-P1-AL-01

Task ID: TH-P1-AL-01
Title: Alipay Config And Startup Validation
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-03
Status: DONE (Batch 7 — Alipay config validation + fail-fast, see execution-logs/TH-P1-AL-01.md)

Objective:

Load and validate Alipay merchant configuration before any live Alipay order can be created.

Current State:

No official Alipay config path exists.

Scope:

Add config fields, validation errors, and redacted diagnostics.

Out of Scope:

CreateOrder, notify, query.

Implementation Notes:

Separate sandbox and production values.

Acceptance Criteria:

- AC-01: Given `payment_channel=alipay` and missing app id, startup or payment info check reports config error.
- AC-02: Given all required Alipay fields are present, payment info can report Alipay method available.
- AC-03: Given config validation fails, logs contain no private key or certificate body.

Test Requirements:

- Unit: config validation table.
- Integration: service info with Alipay channel.
- Regression: epay config behavior remains unchanged.
- Failure Injection: missing key and malformed URL.

Observability Requirements:

Metric config readiness by channel.

Audit Requirements:

Admin config changes use existing audit middleware.

Migration / Rollback Requirements:

No migration unless settings storage is extended; rollback sets channel away from Alipay.

Documentation Requirements:

Update deployment env list for Alipay.

Risks:

Bad key handling can leak merchant secrets.

Definition of Done:

Global DoD applies.

### TH-P1-AL-02

Task ID: TH-P1-AL-02
Title: Alipay CreateOrder Client
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-AL-01

Objective:

Create an Alipay QR/payment order and return a usable payment URL.

Current State:

No official Alipay client exists.

Scope:

Implement client call, request mapping, response parsing, and error mapping for order creation.

Out of Scope:

Callback verification and settlement.

Implementation Notes:

Use decimal amount formatted to two CNY decimals.

Acceptance Criteria:

- AC-01: Given valid sandbox config and amount `0.01`, create order returns non-empty pay URL and local order number.
- AC-02: Given Alipay returns provider error, service returns provider error class and creates no paid order state.
- AC-03: Given context timeout, service returns timeout error and creates no wallet transaction.

Test Requirements:

- Unit: request mapper and amount formatter.
- Integration: mocked Alipay client.
- Regression: local order remains pending after create.
- Failure Injection: provider error and timeout.

Observability Requirements:

Metric for Alipay create outcome and duration.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document sandbox create-order probe.

Risks:

Provider error mapping can hide failed order creation. Keep raw provider code in sanitized metadata.

Definition of Done:

Global DoD applies.

### TH-P1-AL-03

Task ID: TH-P1-AL-03
Title: Alipay Notify Signature Verification
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-AL-01, TH-P1-04

Objective:

Verify Alipay callback signatures and normalize paid callback payloads without wallet mutation.

Current State:

Only epay signature verification exists.

Scope:

Implement Alipay signature verification, amount parsing, trade status mapping, and failure response text.

Out of Scope:

Wallet settlement.

Implementation Notes:

Verification task returns `NotifyResult`; settlement task consumes it.

Acceptance Criteria:

- AC-01: Given valid signed paid callback, verifier returns order number, trade number, amount, method, and success=true.
- AC-02: Given invalid signature, verifier returns signature error and no `NotifyResult`.
- AC-03: Given malformed amount, verifier returns amount parse error and no wallet call.

Test Requirements:

- Unit: signature fixture and amount parser.
- Integration: callback route reaches verifier.
- Regression: epay bad-signature test remains green.
- Failure Injection: invalid signature and malformed amount.

Observability Requirements:

Metric verify success/failure by reason.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document callback verification fields.

Risks:

Accepting unsigned callback is a direct money risk.

Definition of Done:

Global DoD applies.

### TH-P1-AL-04

Task ID: TH-P1-AL-04
Title: Alipay Notify Settlement Integration
Phase: P1
Epic: Payment
Type: Backend / Finance
Priority: P1
Dependencies: TH-P1-AL-03

Objective:

Settle verified Alipay paid callbacks through the existing idempotent payment service.

Current State:

`HandleNotify` settles epay callbacks idempotently. Alipay callback result is not wired.

Scope:

Connect Alipay `NotifyResult` to order lookup, amount check, wallet credit/subscription activation, and `MarkPaid`.

Out of Scope:

Active query and worker.

Implementation Notes:

Reuse `order_no` idempotency key and pending-to-paid compare-and-set.

Acceptance Criteria:

- AC-01: Given verified Alipay paid callback with matching amount, local order changes `pending -> paid`.
- AC-02: Given the same callback twice, exactly one wallet transaction exists.
- AC-03: Given amount mismatch, order remains pending and no wallet transaction is created.
- AC-04: Given local order is expired, order closes and no wallet transaction is created.

Test Requirements:

- Unit: settlement branch mapping.
- Integration: payment service plus wallet repo.
- Regression: epay settlement remains green.
- Failure Injection: duplicate callback, amount mismatch, expired order.

Observability Requirements:

Metric settlement outcome by channel.

Audit Requirements:

No admin audit row for automatic callback.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document Alipay callback settlement behavior.

Risks:

Duplicate callbacks are normal; idempotency must be hard-tested.

Definition of Done:

Global DoD applies.

### TH-P1-AL-05

Task ID: TH-P1-AL-05
Title: Alipay QueryOrder Client
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-AL-01, TH-P1-01

Objective:

Implement Alipay active order inquiry and map provider states to the shared `QueryOrder` result.

Current State:

No active Alipay query exists.

Scope:

Query by local order number, parse provider response, normalize paid/unpaid/closed/unknown/timeout.

Out of Scope:

Worker scan loop.

Implementation Notes:

Timeout returns retryable and mutates no local state.

Acceptance Criteria:

- AC-01: Given Alipay query returns paid, result includes trade number, amount, paid time, and state `paid`.
- AC-02: Given Alipay query returns unpaid, result state is `not_paid` and retryable=false.
- AC-03: Given Alipay query times out, result is retryable and local order remains unchanged.

Test Requirements:

- Unit: state mapper.
- Integration: mocked Alipay query client.
- Regression: settlement intent tests consume Alipay result.
- Failure Injection: timeout and malformed provider response.

Observability Requirements:

Metric query outcome and duration by channel.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document query state mapping.

Risks:

Wrong unpaid/closed mapping can close payable orders.

Definition of Done:

Global DoD applies.

### TH-P1-AL-06

Task ID: TH-P1-AL-06
Title: Alipay Sandbox Integration Test
Phase: P1
Epic: Payment
Type: Test / Provider
Priority: P1
Dependencies: TH-P1-AL-02, TH-P1-AL-04, TH-P1-AL-05

Objective:

Run one Alipay sandbox flow covering create, notify, duplicate notify, amount mismatch, and active query.

Current State:

No official Alipay sandbox test exists.

Scope:

Add sandbox or deterministic mocked integration path and document required credentials.

Out of Scope:

Production real-money run.

Implementation Notes:

Use 0.01 CNY and isolate test order prefix.

Acceptance Criteria:

- AC-01: Given sandbox credentials, create order returns a QR/payment URL.
- AC-02: Given paid sandbox notification is replayed twice, one local wallet credit exists.
- AC-03: Given sandbox query after payment, local order and provider state are both paid.
- AC-04: Given mismatched amount fixture, settlement is rejected and order remains pending.

Test Requirements:

- Integration: sandbox or deterministic provider mock.
- Regression: local payment service tests remain green.
- Manual: record one sandbox run id.
- Failure Injection: duplicate callback and mismatched amount.

Observability Requirements:

Report create/notify/query durations without secrets.

Audit Requirements:

No admin audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Add sandbox run steps and fixtures.

Risks:

Sandbox instability can block CI. Keep deterministic mock path for CI.

Definition of Done:

Global DoD applies.

### TH-P1-AL-07

Task ID: TH-P1-AL-07
Title: Alipay Production Small Amount Verification Runbook
Phase: P1
Epic: Payment
Type: Operations / Provider
Priority: P1
Dependencies: TH-P1-AL-06

Objective:

Create the runbook for one controlled small-value Alipay production verification.

Current State:

No production verification checklist exists.

Scope:

Document pre-checks, 0.01 CNY order, callback, wallet credit, reconciliation evidence, and rollback.

Out of Scope:

Executing the real-money run in this task unless scheduled by release owner.

Implementation Notes:

Require P0.5 gate pass before execution.

Acceptance Criteria:

- AC-01: Given runbook is followed, it records order number, provider trade number, local paid timestamp, and wallet transaction id.
- AC-02: Given callback is not received within window, runbook instructs active query and manual review path.
- AC-03: Given any check fails, runbook instructs channel disablement and leaves epay/new-order state unchanged.

Test Requirements:

- Manual: dry-run checklist review.
- Regression: deployment docs link to runbook.
- Failure Injection: callback missing path.

Observability Requirements:

Runbook references metrics and logs to inspect.

Audit Requirements:

Any admin config switch must be audited.

Migration / Rollback Requirements:

Rollback step disables Alipay channel and keeps callback route deployed for historical order.

Documentation Requirements:

Commit runbook under docs.

Risks:

Real payment verification can move money. Use minimal amount and named operator.

Definition of Done:

Global DoD applies.

## WeChat Pay

### TH-P1-WX-01

Task ID: TH-P1-WX-01
Title: WeChat Pay Config And Startup Validation
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-03

Objective:

Load and validate WeChat Pay merchant configuration before any live WeChat order can be created.

Current State:

No official WeChat Pay config path exists.

Scope:

Add merchant id, app id, API v3 key, certificate serial, private key path/body, notify URL, and validation errors.

Out of Scope:

CreateOrder, callback decrypt, query.

Implementation Notes:

Never log key or certificate contents.

Acceptance Criteria:

- AC-01: Given `payment_channel=wechatpay` and missing merchant id, payment info reports config error.
- AC-02: Given all required fields are present, WeChat method can be listed as available.
- AC-03: Given private key parse fails, log contains error class but no key body.

Test Requirements:

- Unit: config validation table.
- Integration: payment info readiness.
- Regression: epay and Alipay readiness remain isolated.
- Failure Injection: missing key and malformed certificate serial.

Observability Requirements:

Metric config readiness by channel.

Audit Requirements:

Admin config changes use existing audit middleware.

Migration / Rollback Requirements:

No migration unless settings storage is extended.

Documentation Requirements:

Update deployment env list for WeChat Pay.

Risks:

Certificate handling mistakes can fail all callbacks.

Definition of Done:

Global DoD applies.

### TH-P1-WX-02

Task ID: TH-P1-WX-02
Title: WeChat Pay CreateOrder Client
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-WX-01

Objective:

Create a WeChat Native order and return a `code_url`.

Current State:

No official WeChat Pay client exists.

Scope:

Implement request mapping, fen amount conversion, response parsing, and provider error mapping.

Out of Scope:

Callback and query.

Implementation Notes:

Use integer fen for provider calls and decimal CNY for local orders.

Acceptance Criteria:

- AC-01: Given amount `0.01`, provider request amount is integer `1` fen.
- AC-02: Given provider returns `code_url`, create order returns channel `wechatpay`, method `wxpay`, and non-empty pay URL.
- AC-03: Given provider timeout, service returns timeout error and creates no wallet transaction.

Test Requirements:

- Unit: CNY/fen conversion.
- Integration: mocked WeChat client.
- Regression: local order remains pending after create.
- Failure Injection: provider error and timeout.

Observability Requirements:

Metric WeChat create outcome and duration.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document Native create-order probe.

Risks:

Amount conversion errors can create financial mismatch.

Definition of Done:

Global DoD applies.

### TH-P1-WX-03

Task ID: TH-P1-WX-03
Title: WeChat Pay Notify Signature And Decrypt Verification
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-WX-01, TH-P1-04

Objective:

Verify WeChat Pay callback signatures, decrypt resource payloads, and normalize callback results without wallet mutation.

Current State:

No WeChat callback verifier exists.

Scope:

Implement signature check, timestamp/nonce validation, resource decrypt, amount parsing, and trade state mapping.

Out of Scope:

Wallet settlement.

Implementation Notes:

Raw encrypted payload and decrypted payer data must not be logged.

Acceptance Criteria:

- AC-01: Given valid signed paid callback, verifier returns order number, trade number, fen amount converted to CNY, and success=true.
- AC-02: Given invalid signature, verifier returns signature error and no `NotifyResult`.
- AC-03: Given decrypt fails, verifier returns decrypt error and no wallet call.

Test Requirements:

- Unit: signature/decrypt fixtures.
- Integration: callback route reaches verifier.
- Regression: other provider callback tests remain green.
- Failure Injection: invalid signature, stale timestamp, decrypt failure.

Observability Requirements:

Metric verify success/failure by reason.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document callback verification and redaction.

Risks:

Decrypt mistakes can leak payer data or reject valid callbacks.

Definition of Done:

Global DoD applies.

### TH-P1-WX-04

Task ID: TH-P1-WX-04
Title: WeChat Pay Notify Settlement Integration
Phase: P1
Epic: Payment
Type: Backend / Finance
Priority: P1
Dependencies: TH-P1-WX-03

Objective:

Settle verified WeChat paid callbacks through the existing idempotent payment service.

Current State:

WeChat callback result is not wired to payment settlement.

Scope:

Connect verified result to order lookup, amount check, wallet credit/subscription activation, and `MarkPaid`.

Out of Scope:

Active query and worker.

Implementation Notes:

Use pending-to-paid compare-and-set and `order_no` idempotency.

Acceptance Criteria:

- AC-01: Given verified WeChat paid callback with matching amount, local order changes `pending -> paid`.
- AC-02: Given duplicate callback, exactly one wallet transaction exists.
- AC-03: Given amount mismatch, order remains pending and no wallet transaction is created.
- AC-04: Given expired order, order closes and no wallet transaction is created.

Test Requirements:

- Unit: settlement branch mapping.
- Integration: payment service plus wallet repo.
- Regression: epay and Alipay settlement paths remain green.
- Failure Injection: duplicate callback, amount mismatch, expired order.

Observability Requirements:

Metric settlement outcome by channel.

Audit Requirements:

No admin audit row for automatic callback.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document WeChat callback settlement behavior.

Risks:

Duplicate callbacks are expected; wallet idempotency must be verified.

Definition of Done:

Global DoD applies.

### TH-P1-WX-05

Task ID: TH-P1-WX-05
Title: WeChat Pay QueryOrder Client
Phase: P1
Epic: Payment
Type: Backend / Provider Adapter
Priority: P1
Dependencies: TH-P1-WX-01, TH-P1-01

Objective:

Implement WeChat active order inquiry and map trade states to the shared `QueryOrder` result.

Current State:

No WeChat query exists.

Scope:

Query by local order number and normalize success, not paid, closed, revoked, unknown, and timeout states.

Out of Scope:

Worker scan loop.

Implementation Notes:

Treat timeout as retryable and never as paid.

Acceptance Criteria:

- AC-01: Given query returns `SUCCESS`, result state is `paid` and includes provider trade number and amount.
- AC-02: Given query returns `NOTPAY`, result state is `not_paid` and local order remains unchanged.
- AC-03: Given query times out, result is retryable and local order remains unchanged.

Test Requirements:

- Unit: trade state mapper.
- Integration: mocked query client.
- Regression: settlement intent tests consume WeChat result.
- Failure Injection: timeout and malformed provider response.

Observability Requirements:

Metric query outcome and duration by channel.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document WeChat state mapping.

Risks:

Closing unpaid orders too early can block payment completion.

Definition of Done:

Global DoD applies.

### TH-P1-WX-06

Task ID: TH-P1-WX-06
Title: WeChat Pay Sandbox Integration Test
Phase: P1
Epic: Payment
Type: Test / Provider
Priority: P1
Dependencies: TH-P1-WX-02, TH-P1-WX-04, TH-P1-WX-05

Objective:

Run one WeChat sandbox flow covering create, notify, duplicate notify, amount mismatch, decrypt failure, and query.

Current State:

No official WeChat sandbox test exists.

Scope:

Add sandbox or deterministic mocked integration path and document required credentials.

Out of Scope:

Production real-money run.

Implementation Notes:

Use 0.01 CNY and isolated order prefix.

Acceptance Criteria:

- AC-01: Given sandbox credentials, create order returns `code_url`.
- AC-02: Given paid callback replayed twice, one local wallet credit exists.
- AC-03: Given query after payment, local order and provider state are both paid.
- AC-04: Given decrypt failure fixture, settlement is rejected and order remains pending.

Test Requirements:

- Integration: sandbox or deterministic provider mock.
- Regression: local payment service tests remain green.
- Manual: record one sandbox run id.
- Failure Injection: duplicate callback, mismatched amount, decrypt failure.

Observability Requirements:

Report create/notify/query durations without secrets.

Audit Requirements:

No admin audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Add sandbox run steps and fixtures.

Risks:

Sandbox instability can block CI. Keep deterministic mock path for CI.

Definition of Done:

Global DoD applies.

### TH-P1-WX-07

Task ID: TH-P1-WX-07
Title: WeChat Pay Production Small Amount Verification Runbook
Phase: P1
Epic: Payment
Type: Operations / Provider
Priority: P1
Dependencies: TH-P1-WX-06

Objective:

Create the runbook for one controlled small-value WeChat Pay production verification.

Current State:

No production verification checklist exists.

Scope:

Document pre-checks, 0.01 CNY order, callback/decrypt, wallet credit, reconciliation evidence, and rollback.

Out of Scope:

Executing the real-money run in this task unless scheduled by release owner.

Implementation Notes:

Require P0.5 gate pass before execution.

Acceptance Criteria:

- AC-01: Given runbook is followed, it records order number, provider trade number, local paid timestamp, and wallet transaction id.
- AC-02: Given callback is not received within window, runbook instructs active query and manual review path.
- AC-03: Given any check fails, runbook instructs channel disablement and leaves epay/new-order state unchanged.

Test Requirements:

- Manual: dry-run checklist review.
- Regression: deployment docs link to runbook.
- Failure Injection: callback missing path.

Observability Requirements:

Runbook references metrics and logs to inspect.

Audit Requirements:

Any admin config switch must be audited.

Migration / Rollback Requirements:

Rollback step disables WeChat channel and keeps callback route deployed for historical order.

Documentation Requirements:

Commit runbook under docs.

Risks:

Real payment verification can move money. Use minimal amount and named operator.

Definition of Done:

Global DoD applies.

## Compensation Worker

### TH-P1-CW-01

Task ID: TH-P1-CW-01
Title: Pending Payment Order Scanner
Phase: P1
Epic: Payment
Type: Worker
Priority: P1
Dependencies: TH-P1-05

Objective:

Find payment orders eligible for active query without mutating order or wallet state.

Current State:

No pending-order scanner exists.

Scope:

Select pending orders by channel, age, expiry, retry metadata, and limit.

Out of Scope:

Provider query execution and settlement.

Implementation Notes:

Use deterministic ordering and small batches.

Acceptance Criteria:

- AC-01: Given pending order older than eligibility threshold, scanner returns it.
- AC-02: Given paid/closed/refunded order, scanner excludes it.
- AC-03: Given pending order not yet eligible by retry time, scanner excludes it.

Test Requirements:

- Unit: eligibility rules.
- Integration: Postgres scanner query.
- Regression: payment order list remains unchanged.
- Failure Injection: null retry metadata.

Observability Requirements:

Metric scanned and eligible counts by channel.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

Depends on metadata migration only if TH-P1-05 adds one.

Documentation Requirements:

Document scanner eligibility.

Risks:

Too broad scanner can overload provider query APIs.

Definition of Done:

Global DoD applies.

### TH-P1-CW-02

Task ID: TH-P1-CW-02
Title: Provider Query Dispatcher
Phase: P1
Epic: Payment
Type: Worker
Priority: P1
Dependencies: TH-P1-CW-01, TH-P1-AL-05, TH-P1-WX-05

Objective:

Dispatch eligible orders to the correct provider `QueryOrder` implementation by local order channel.

Current State:

Scanner and query clients are separate; no dispatcher exists.

Scope:

Add dispatcher with channel validation, timeout, and sanitized errors.

Out of Scope:

Settlement and retry persistence.

Implementation Notes:

Route by persisted order channel, not current global channel.

Acceptance Criteria:

- AC-01: Given Alipay order, dispatcher calls Alipay query client exactly once.
- AC-02: Given WeChat order, dispatcher calls WeChat query client exactly once.
- AC-03: Given unsupported channel, dispatcher records manual-review reason and performs no wallet call.

Test Requirements:

- Unit: channel dispatch table.
- Integration: fake clients with scanner output.
- Regression: epay historical behavior remains isolated.
- Failure Injection: provider timeout and unsupported channel.

Observability Requirements:

Metric query dispatched by channel and outcome.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document dispatch rules.

Risks:

Using global channel can corrupt historical order handling.

Definition of Done:

Global DoD applies.

### TH-P1-CW-03

Task ID: TH-P1-CW-03
Title: Paid Order Compensation
Phase: P1
Epic: Payment
Type: Worker / Finance
Priority: P1
Dependencies: TH-P1-CW-02, TH-P1-02

Objective:

Settle provider-confirmed paid pending orders through the existing idempotent payment service.

Current State:

Paid callbacks settle orders, but query-confirmed paid orders do not.

Scope:

Apply pending-to-paid transition and wallet credit/subscription activation using existing payment settlement invariants.

Out of Scope:

Retry/backoff and closed-order handling.

Implementation Notes:

Use `order_no` idempotency key; repeat processing must not move money twice.

Acceptance Criteria:

- AC-01: Given provider query says paid with matching amount, worker marks order paid and creates one wallet transaction.
- AC-02: Given the same order is processed twice, one wallet transaction exists.
- AC-03: Given amount mismatch, worker leaves order pending and creates no wallet transaction.

Test Requirements:

- Unit: paid-intent branch.
- Integration: worker plus wallet repository.
- Regression: callback duplicate tests remain green.
- Concurrency: two workers process same paid order.

Observability Requirements:

Metric paid compensation count and amount bucket.

Audit Requirements:

Automatic worker action writes operational evidence, not admin audit.

Migration / Rollback Requirements:

No migration. Rollback disables paid compensation worker path.

Documentation Requirements:

Document idempotency behavior.

Risks:

False paid mapping causes wrongful credit. Depend on provider query tests.

Definition of Done:

Global DoD applies.

### TH-P1-CW-04

Task ID: TH-P1-CW-04
Title: Closed And Expired Order Handling
Phase: P1
Epic: Payment
Type: Worker
Priority: P1
Dependencies: TH-P1-CW-02

Objective:

Close local pending orders when provider confirms terminal closed state or local expiry has passed.

Current State:

Orders can be marked closed on expired callback, but no worker closes stale pending orders.

Scope:

Handle provider closed/revoked states and local expiry.

Out of Scope:

Paid settlement and retry backoff.

Implementation Notes:

Do not close retryable timeout states.

Acceptance Criteria:

- AC-01: Given provider state closed, worker marks local pending order closed.
- AC-02: Given local expiry passed and provider not paid, worker marks local order closed.
- AC-03: Given provider timeout, worker leaves order pending.

Test Requirements:

- Unit: terminal state mapping.
- Integration: order status update.
- Regression: paid orders are never closed by scanner.
- Failure Injection: provider timeout.

Observability Requirements:

Metric closed by provider and closed by expiry.

Audit Requirements:

No admin audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document close rules.

Risks:

Closing too early can block delayed payment. Respect provider state and expiry.

Definition of Done:

Global DoD applies.

### TH-P1-CW-05

Task ID: TH-P1-CW-05
Title: Retry And Backoff Metadata
Phase: P1
Epic: Payment
Type: Worker
Priority: P1
Dependencies: TH-P1-CW-02

Objective:

Persist retry count, last query time, next query time, and last sanitized failure reason for payment order compensation.

Current State:

No retry metadata exists unless TH-P1-05 introduces it.

Scope:

Add metadata persistence and backoff calculation for retryable query results.

Out of Scope:

Manual review UI.

Implementation Notes:

Use additive nullable migration if needed.

Acceptance Criteria:

- AC-01: Given provider timeout, retry count increments by one and next query time moves forward.
- AC-02: Given retry count is below max, order remains pending.
- AC-03: Given retry metadata update fails, order status and wallet are unchanged.

Test Requirements:

- Unit: backoff calculator.
- Integration: retry metadata persistence.
- Regression: old order rows with null metadata are eligible under defaults.
- Failure Injection: DB update failure.

Observability Requirements:

Metric retryable query failures by channel.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

Provide down migration if columns/table are added.

Documentation Requirements:

Document retry policy and max retry.

Risks:

Aggressive retry can hit provider rate limits.

Definition of Done:

Global DoD applies.

### TH-P1-CW-06

Task ID: TH-P1-CW-06
Title: Manual Review Escalation
Phase: P1
Epic: Payment
Type: Worker / Review
Priority: P1
Dependencies: TH-P1-CW-05

Objective:

Escalate payment orders to manual review when max retry or unsafe query result is reached.

Current State:

Admin manual completion exists, but compensation review queue does not.

Scope:

Mark or create review item for max retries, unsupported channel, amount mismatch, and unknown provider state.

Out of Scope:

Admin review UI and wallet adjustment workflows.

Implementation Notes:

Manual review must not auto-credit or auto-debit.

Acceptance Criteria:

- AC-01: Given retry count reaches max, worker creates review item and leaves order pending.
- AC-02: Given amount mismatch, worker creates review item and performs no wallet call.
- AC-03: Given same review condition repeats, exactly one open review item exists for the order.

Test Requirements:

- Unit: escalation classifier.
- Integration: review item idempotency.
- Regression: admin manual complete still works only when invoked by admin.
- Failure Injection: duplicate worker escalation.

Observability Requirements:

Metric payment review items by reason.

Audit Requirements:

Review item creation records system actor evidence.

Migration / Rollback Requirements:

Add review storage only if no reusable table exists; include down migration.

Documentation Requirements:

Document manual review reasons.

Risks:

Missing review item can hide payment drift.

Definition of Done:

Global DoD applies.

### TH-P1-CW-07

Task ID: TH-P1-CW-07
Title: Worker Lease And Idempotency
Phase: P1
Epic: Payment
Type: Worker / Reliability
Priority: P1
Dependencies: TH-P1-CW-03, TH-P1-CW-04

Objective:

Run payment compensation under Redis lease and protect each order mutation from duplicate worker execution.

Current State:

Existing workers use lease; payment compensation worker does not exist.

Scope:

Add worker registration, lease key, idempotent order locking, and duplicate-consumption tests.

Out of Scope:

Provider client internals.

Implementation Notes:

Use row-level compare-and-set and wallet idempotency.

Acceptance Criteria:

- AC-01: Given two worker instances, one acquires payment compensation lease and the other records skipped.
- AC-02: Given two workers race on same paid order, one marks paid and one observes no-op.
- AC-03: Given Redis lease error, compensation cycle is skipped and order remains pending.

Test Requirements:

- Unit: lease key naming.
- Integration: Redis lease and Postgres order CAS.
- Regression: existing worker leases remain unchanged.
- Concurrency: duplicate worker race.

Observability Requirements:

Metric compensation worker acquired/skipped/failed.

Audit Requirements:

No admin audit row.

Migration / Rollback Requirements:

No migration. Rollback unregisters payment compensation worker.

Documentation Requirements:

Add worker name and lease key to deployment docs.

Risks:

Duplicate workers can double-credit if CAS or wallet idempotency is bypassed.

Definition of Done:

Global DoD applies.

### TH-P1-CW-08

Task ID: TH-P1-CW-08
Title: Compensation Metrics
Phase: P1
Epic: Payment
Type: Observability
Priority: P1
Dependencies: TH-P1-CW-05, TH-P05-04

Objective:

Expose compensation worker metrics for scan, query, paid, closed, retry, review, and failure outcomes.

Current State:

No payment compensation metrics exist.

Scope:

Add bounded metrics and tests.

Out of Scope:

Alert pack beyond P0.5 basics.

Implementation Notes:

Labels: channel, outcome, reason. No order number or trade number labels.

Acceptance Criteria:

- AC-01: Given one eligible order is scanned, scanned and eligible counters increment.
- AC-02: Given provider query timeout, retry counter increments with channel label.
- AC-03: Given review escalation, review counter increments with reason label.

Test Requirements:

- Unit: label sanitizer.
- Integration: worker emits counters.
- Regression: P0.5 metrics still expose billing events.
- Failure Injection: timeout and amount mismatch.

Observability Requirements:

This task defines payment compensation metric names.

Audit Requirements:

No audit row.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document metrics.

Risks:

Order ids in labels can overload metrics. Tests must reject them.

Definition of Done:

Global DoD applies.

### TH-P1-CW-09

Task ID: TH-P1-CW-09
Title: Compensation Failure And Restart Integration Tests
Phase: P1
Epic: Payment
Type: Test / Reliability
Priority: P1
Dependencies: TH-P1-CW-03, TH-P1-CW-04, TH-P1-CW-05, TH-P1-CW-07

Objective:

Verify compensation behavior across timeout, DB failure, worker restart, and duplicate consumption.

Current State:

Individual compensation parts are split; integrated failure tests are missing.

Scope:

Add focused end-to-end worker tests with fake provider and real Postgres/Redis where available.

Out of Scope:

Sandbox provider calls.

Implementation Notes:

Keep scenarios deterministic and isolated by order prefix.

Acceptance Criteria:

- AC-01: Given worker stops after provider paid result before local mutation, next run settles order once.
- AC-02: Given DB failure after wallet credit before mark-paid, transaction rollback leaves one consistent state and retry does not double-credit.
- AC-03: Given duplicate worker consumption, final order status and wallet ledger match one settlement.

Test Requirements:

- Integration: Postgres/Redis worker flow.
- Regression: callback settlement tests remain green.
- Concurrency: duplicate worker scenario.
- Failure Injection: worker restart and DB failure hooks.

Observability Requirements:

Tests assert failure metrics where emitted.

Audit Requirements:

Review item evidence is asserted when escalation occurs.

Migration / Rollback Requirements:

No migration.

Documentation Requirements:

Document restart safety guarantees.

Risks:

Flaky timing can weaken trust. Prefer explicit hooks over sleeps.

Definition of Done:

Global DoD applies.

## Payment Reconciliation And Epay Cutover

### TH-P1-RC-01

Task ID: TH-P1-RC-01
Title: Provider Payment Bill Import Contract
Phase: P1
Epic: Payment
Type: Finance / Contract
Priority: P1
Dependencies: TH-P1-AL-06, TH-P1-WX-06

Objective:

Define normalized provider payment bill rows for Alipay and WeChat paid-order reconciliation.

Current State:

Gateway usage billing sync exists, but payment provider bill import does not.

Scope:

Define fields for provider, trade number, local order number, amount, paid time, fee, and raw reference.

Out of Scope:

Reconciliation UI and epay cutover.

Implementation Notes:

Keep raw payload reference sanitized.

Acceptance Criteria:

- AC-01: Given Alipay bill fixture, importer maps local order number, amount, paid time, and trade number.
- AC-02: Given WeChat bill fixture, importer maps local order number, fen amount as CNY, paid time, and trade number.
- AC-03: Given malformed bill row, importer rejects row and records sanitized error.

Test Requirements:

- Unit: fixture mappers.
- Integration: importer persistence if storage is added.
- Regression: existing billing_records sync remains untouched.
- Failure Injection: malformed row.

Observability Requirements:

Metric imported, rejected, and duplicate bill rows by provider.

Audit Requirements:

No admin audit row.

Migration / Rollback Requirements:

Add storage migration only if needed; include down migration.

Documentation Requirements:

Document normalized bill row.

Risks:

Timezone mistakes can create false reconciliation diffs.

Definition of Done:

Global DoD applies.

### TH-P1-RC-02

Task ID: TH-P1-RC-02
Title: Payment Reconciliation Diff Classifier
Phase: P1
Epic: Payment
Type: Finance / Reconciliation
Priority: P1
Dependencies: TH-P1-RC-01

Objective:

Classify provider payment bill rows against local orders and wallet top-up ledger without mutating wallets.

Current State:

No payment-order reconciliation classifier exists.

Scope:

Classify provider_paid_local_pending, local_paid_missing_provider, amount_mismatch, missing_wallet_topup, duplicate_trade_no, and balanced.

Out of Scope:

Automatic wallet changes and review UI.

Implementation Notes:

Classifier returns diffs only.

Acceptance Criteria:

- AC-01: Given provider paid and local pending, classifier returns `provider_paid_local_pending`.
- AC-02: Given local paid and no provider paid row, classifier returns `local_paid_missing_provider`.
- AC-03: Given provider/local paid and one matching wallet top-up, classifier returns `balanced`.
- AC-04: Given amount mismatch, classifier returns diff and performs no wallet call.

Test Requirements:

- Unit: diff classifier table.
- Integration: fixture query over payment_orders and wallet_transactions.
- Regression: existing usage reconciliation remains unchanged.
- Failure Injection: duplicate trade number.

Observability Requirements:

Metric diffs by type and provider.

Audit Requirements:

No automatic money audit row.

Migration / Rollback Requirements:

No migration unless diff table extension is needed.

Documentation Requirements:

Document payment diff types.

Risks:

Classifier must not hide missing ledger rows.

Definition of Done:

Global DoD applies.

### TH-P1-RC-03

Task ID: TH-P1-RC-03
Title: Payment Reconciliation Run Persistence
Phase: P1
Epic: Payment
Type: Finance / Reconciliation
Priority: P1
Dependencies: TH-P1-RC-02

Objective:

Persist payment reconciliation runs and diffs in a queryable form without wallet mutation.

Current State:

Usage reconciliation persists runs/diffs; payment reconciliation does not.

Scope:

Reuse or extend reconciliation storage for payment diff runs and summaries.

Out of Scope:

Admin UI and adjustment workflow.

Implementation Notes:

Do not overwrite provider bill, usage, order, or wallet ledger rows.

Acceptance Criteria:

- AC-01: Given classifier returns two diffs, run persistence writes one run and two diff rows.
- AC-02: Given classifier returns balanced for all rows, run persistence writes completed run with zero diffs.
- AC-03: Given diff persistence fails, run is marked failed or partial according to documented rule.

Test Requirements:

- Unit: run summary builder.
- Integration: run and diff persistence.
- Regression: existing reconciliation run list still reads old runs.
- Failure Injection: diff write failure.

Observability Requirements:

Metric payment reconciliation run status and diff count.

Audit Requirements:

No automatic money audit row.

Migration / Rollback Requirements:

Migration with down script if schema changes.

Documentation Requirements:

Document payment reconciliation storage.

Risks:

Mixing payment and usage diffs without type fields can confuse operators.

Definition of Done:

Global DoD applies.

### TH-P1-RC-04

Task ID: TH-P1-RC-04
Title: Epay New Order Disable Switch
Phase: P1
Epic: Payment
Type: Backend / Cutover
Priority: P1
Dependencies: TH-P1-AL-07, TH-P1-WX-07

Objective:

Add a reversible switch that blocks new epay orders after official channels pass verification.

Current State:

epay remains available when configured.

Scope:

Block new epay order creation while leaving existing epay order reads untouched.

Out of Scope:

Deleting epay code or dependency.

Implementation Notes:

Switch must be reversible during observation window.

Acceptance Criteria:

- AC-01: Given switch is enabled, new epay order creation returns disabled-channel error and creates no order row.
- AC-02: Given switch is disabled, epay order creation follows existing behavior.
- AC-03: Given switch changes through admin settings, audit log records actor and old/new value.

Test Requirements:

- Unit: switch rule.
- Integration: order creation blocked/unblocked.
- Regression: official channel creation unaffected.
- Manual: cutover checklist toggle.

Observability Requirements:

Metric epay blocked order attempts.

Audit Requirements:

Admin switch mutation must be audited.

Migration / Rollback Requirements:

No migration unless settings key is added; rollback disables switch.

Documentation Requirements:

Document cutover switch.

Risks:

Blocking epay before official channel readiness can stop充值.

Definition of Done:

Global DoD applies.

### TH-P1-RC-05

Task ID: TH-P1-RC-05
Title: Epay Historical Callback Window
Phase: P1
Epic: Payment
Type: Backend / Cutover
Priority: P1
Dependencies: TH-P1-RC-04, TH-P1-04

Objective:

Keep historical epay pending orders settleable after new epay order creation is disabled.

Current State:

epay callback route exists; cutover behavior is not specified.

Scope:

Allow valid callbacks for existing epay orders until observation window ends.

Out of Scope:

Physical epay removal.

Implementation Notes:

Route by local order channel and order creation timestamp.

Acceptance Criteria:

- AC-01: Given existing pending epay order and valid callback inside window, order settles paid once.
- AC-02: Given new epay order creation is disabled, callback route still handles old pending order.
- AC-03: Given callback arrives after observation window, route creates manual-review evidence and does not auto-settle.

Test Requirements:

- Unit: observation window rule.
- Integration: old epay callback after disable switch.
- Regression: duplicate epay callback remains idempotent.
- Failure Injection: late callback.

Observability Requirements:

Metric epay historical callbacks by outcome.

Audit Requirements:

Late callback review evidence is recorded.

Migration / Rollback Requirements:

No migration. Rollback reopens epay new-order switch if needed.

Documentation Requirements:

Document observation window.

Risks:

Too short window can strand legitimate payers.

Definition of Done:

Global DoD applies.

### TH-P1-RC-06

Task ID: TH-P1-RC-06
Title: Payment Cutover Runbook
Phase: P1
Epic: Payment
Type: Operations / Cutover
Priority: P1
Dependencies: TH-P1-RC-03, TH-P1-RC-05

Objective:

Create the operational runbook for moving from epay M0 to official payment channels.

Current State:

No complete payment cutover runbook exists.

Scope:

Document readiness checks, channel switch, small amount verification, monitoring window, epay disablement, historical callback window, and rollback.

Out of Scope:

Provider adapter code.

Implementation Notes:

Payment cutover cannot start unless TH-P05-09 passes.

Acceptance Criteria:

- AC-01: Given runbook pre-check is executed, it requires P0.5 gate report and both provider verification reports.
- AC-02: Given official channel fails during observation, rollback steps restore previous working channel without deleting pending orders.
- AC-03: Given epay disablement is complete, runbook lists how to verify no new epay orders are created.

Test Requirements:

- Manual: tabletop review with engineering and finance.
- Regression: deployment docs link to runbook.
- Failure Injection: callback loss and provider outage decision path.

Observability Requirements:

Runbook references metrics and alert names.

Audit Requirements:

All admin setting changes during cutover must be audited.

Migration / Rollback Requirements:

Runbook contains rollback plan and pending-order handling.

Documentation Requirements:

Commit runbook and link from `docs/DEPLOYMENT.md`.

Risks:

Cutover without rollback can block customer充值.

Definition of Done:

Global DoD applies.

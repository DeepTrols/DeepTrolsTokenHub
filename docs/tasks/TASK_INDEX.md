# Sprint Task Index

> 本索引于 2026-09-02 由 `docs/tasks/` 六个阶段任务文件（P0.5–P5，唯一事实来源）
> 程序化重新生成，取代第一轮拆分残留。共 138 个 Sprint Task。
> Estimate 为 `—` 表示任务文件未定义工作量（P0.5 / P1）；任务文件均无 Status
> 字段，状态一律默认 TODO。Epic/Feature 不进索引，仅收录 `### TH-*` Sprint Task。

| ID | Title | Phase | Epic | Type | Priority | Dependencies | Estimate | Status |
|---|---|---|---|---|---|---|---|---|
| TH-P05-01 | B5 Reserve Maximum Charge Calculation | P0.5 | Production Safety Gate | Backend / Billing | P0 | None | — | TODO |
| TH-P05-02 | B5 Settle Fallback Visibility Correction | P0.5 | Production Safety Gate | Backend / Billing | P0 | TH-P05-01 | — | TODO |
| TH-P05-03 | Billing Invariant And Concurrency Tests | P0.5 | Production Safety Gate | Test | P0 | TH-P05-01, TH-P05-02 | — | TODO |
| TH-P05-04 | Basic Gateway Billing Metrics | P0.5 | Production Safety Gate | Observability | P0 | TH-P05-02 | — | TODO |
| TH-P05-05 | Basic Production Alerts | P0.5 | Production Safety Gate | Observability | P0 | TH-P05-04 | — | TODO |
| TH-P05-06 | Database Backup Baseline | P0.5 | Production Safety Gate | Operations | P0 | None | — | TODO |
| TH-P05-07 | Backup Restore Drill | P0.5 | Production Safety Gate | Operations | P0 | TH-P05-06 | — | TODO |
| TH-P05-08 | Clean Environment Deployment Verification | P0.5 | Production Safety Gate | Operations / Deployment | P0 | TH-P05-07 | — | TODO |
| TH-P05-09 | Production Safety Gate Harness | P0.5 | Production Safety Gate | Test Harness | P0 | TH-P05-03, TH-P05-05, TH-P05-08, TH-P05-10, TH-P05-11 | — | TODO |
| TH-P05-10 | Payment Order PayURL Persistence Fix | P0.5 | Production Safety Gate | Bug Fix | P0 | None | — | TODO |
| TH-P05-11 | Worker Lease Observability | P0.5 | Production Safety Gate | Observability | P0 | TH-P05-04 | — | TODO |
| TH-P1-01 | QueryOrder Result Contract | P1 | Payment | Backend / Contract | P1 | TH-P05-09 | — | TODO |
| TH-P1-02 | QueryOrder Settlement Intent Service | P1 | Payment | Backend / Service | P1 | TH-P1-01 | — | TODO |
| TH-P1-03 | Payment Channel Factory | P1 | Payment | Backend / Integration | P1 | TH-P1-01 | — | TODO |
| TH-P1-04 | Callback Route Channel Resolver | P1 | Payment | Backend / Routing | P1 | TH-P1-03 | — | TODO |
| TH-P1-05 | Payment Order Provider Metadata | P1 | Payment | Backend / Data Contract | P1 | TH-P1-03, TH-P05-10 | — | TODO |
| TH-P1-AL-01 | Alipay Config And Startup Validation | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-03 | — | TODO |
| TH-P1-AL-02 | Alipay CreateOrder Client | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-AL-01 | — | TODO |
| TH-P1-AL-03 | Alipay Notify Signature Verification | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-AL-01, TH-P1-04 | — | TODO |
| TH-P1-AL-04 | Alipay Notify Settlement Integration | P1 | Payment | Backend / Finance | P1 | TH-P1-AL-03 | — | TODO |
| TH-P1-AL-05 | Alipay QueryOrder Client | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-AL-01, TH-P1-01 | — | TODO |
| TH-P1-AL-06 | Alipay Sandbox Integration Test | P1 | Payment | Test / Provider | P1 | TH-P1-AL-02, TH-P1-AL-04, TH-P1-AL-05 | — | TODO |
| TH-P1-AL-07 | Alipay Production Small Amount Verification Runbook | P1 | Payment | Operations / Provider | P1 | TH-P1-AL-06 | — | TODO |
| TH-P1-WX-01 | WeChat Pay Config And Startup Validation | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-03 | — | TODO |
| TH-P1-WX-02 | WeChat Pay CreateOrder Client | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-WX-01 | — | TODO |
| TH-P1-WX-03 | WeChat Pay Notify Signature And Decrypt Verification | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-WX-01, TH-P1-04 | — | TODO |
| TH-P1-WX-04 | WeChat Pay Notify Settlement Integration | P1 | Payment | Backend / Finance | P1 | TH-P1-WX-03 | — | TODO |
| TH-P1-WX-05 | WeChat Pay QueryOrder Client | P1 | Payment | Backend / Provider Adapter | P1 | TH-P1-WX-01, TH-P1-01 | — | TODO |
| TH-P1-WX-06 | WeChat Pay Sandbox Integration Test | P1 | Payment | Test / Provider | P1 | TH-P1-WX-02, TH-P1-WX-04, TH-P1-WX-05 | — | TODO |
| TH-P1-WX-07 | WeChat Pay Production Small Amount Verification Runbook | P1 | Payment | Operations / Provider | P1 | TH-P1-WX-06 | — | TODO |
| TH-P1-CW-01 | Pending Payment Order Scanner | P1 | Payment | Worker | P1 | TH-P1-05 | — | TODO |
| TH-P1-CW-02 | Provider Query Dispatcher | P1 | Payment | Worker | P1 | TH-P1-CW-01, TH-P1-AL-05, TH-P1-WX-05 | — | TODO |
| TH-P1-CW-03 | Paid Order Compensation | P1 | Payment | Worker / Finance | P1 | TH-P1-CW-02, TH-P1-02 | — | TODO |
| TH-P1-CW-04 | Closed And Expired Order Handling | P1 | Payment | Worker | P1 | TH-P1-CW-02 | — | TODO |
| TH-P1-CW-05 | Retry And Backoff Metadata | P1 | Payment | Worker | P1 | TH-P1-CW-02 | — | TODO |
| TH-P1-CW-06 | Manual Review Escalation | P1 | Payment | Worker / Review | P1 | TH-P1-CW-05 | — | TODO |
| TH-P1-CW-07 | Worker Lease And Idempotency | P1 | Payment | Worker / Reliability | P1 | TH-P1-CW-03, TH-P1-CW-04 | — | TODO |
| TH-P1-CW-08 | Compensation Metrics | P1 | Payment | Observability | P1 | TH-P1-CW-05, TH-P05-04 | — | TODO |
| TH-P1-CW-09 | Compensation Failure And Restart Integration Tests | P1 | Payment | Test / Reliability | P1 | TH-P1-CW-03, TH-P1-CW-04, TH-P1-CW-05, TH-P1-CW-07 | — | TODO |
| TH-P1-RC-01 | Provider Payment Bill Import Contract | P1 | Payment | Finance / Contract | P1 | TH-P1-AL-06, TH-P1-WX-06 | — | TODO |
| TH-P1-RC-02 | Payment Reconciliation Diff Classifier | P1 | Payment | Finance / Reconciliation | P1 | TH-P1-RC-01 | — | TODO |
| TH-P1-RC-03 | Payment Reconciliation Run Persistence | P1 | Payment | Finance / Reconciliation | P1 | TH-P1-RC-02 | — | TODO |
| TH-P1-RC-04 | Epay New Order Disable Switch | P1 | Payment | Backend / Cutover | P1 | TH-P1-AL-07, TH-P1-WX-07 | — | TODO |
| TH-P1-RC-05 | Epay Historical Callback Window | P1 | Payment | Backend / Cutover | P1 | TH-P1-RC-04, TH-P1-04 | — | TODO |
| TH-P1-RC-06 | Payment Cutover Runbook | P1 | Payment | Operations / Cutover | P1 | TH-P1-RC-03, TH-P1-RC-05 | — | TODO |
| TH-P2-01 | Enterprise Application Validation | P2 | Enterprise | Backend / API | P1 | TH-P05-09 | 1d | TODO |
| TH-P2-02 | Enterprise Application Transaction | P2 | Enterprise | Backend / Persistence | P1 | TH-P2-01 | 1.5d | TODO |
| TH-P2-03 | Enterprise Duplicate Application Guard | P2 | Enterprise | Backend / Validation | P1 | TH-P2-02 | 1d | TODO |
| TH-P2-04 | Enterprise Self-Service Read API | P2 | Enterprise | Backend / API | P1 | TH-P2-02 | 1d | TODO |
| TH-P2-05 | Enterprise Review Decision API | P2 | Enterprise | Backend / Admin API | P1 | TH-P2-02 | 1.5d | TODO |
| TH-P2-06 | Enterprise Review Audit Snapshot | P2 | Enterprise | Audit / Backend | P1 | TH-P2-05 | 1d | TODO |
| TH-P2-INV-01 | Invitation Schema Gap Review | P2 | Enterprise | Backend / Design | P1 | TH-P2-02 | 0.5d | TODO |
| TH-P2-INV-02 | Create Invitation API | P2 | Enterprise | Backend / API | P1 | TH-P2-INV-01, TH-P2-05 | 1d | TODO |
| TH-P2-INV-03 | List And Revoke Invitation API | P2 | Enterprise | Backend / API | P1 | TH-P2-INV-02 | 1d | TODO |
| TH-P2-INV-04 | Accept Invitation Token API | P2 | Enterprise | Backend / API | P1 | TH-P2-INV-02 | 1.5d | TODO |
| TH-P2-INV-05 | Membership Role Update Guard | P2 | Enterprise | Backend / Authorization | P1 | TH-P2-INV-04 | 1d | TODO |
| TH-P2-INV-06 | Member Removal Owner Guard | P2 | Enterprise | Backend / Authorization | P1 | TH-P2-INV-04 | 1d | TODO |
| TH-P2-WAL-01 | Enterprise Wallet Holder Design Record | P2 | Enterprise | Finance / Design | P1 | TH-P2-05 | 0.5d | TODO |
| TH-P2-WAL-02 | Enterprise Wallet Migration | P2 | Enterprise | Finance / Migration | P1 | TH-P2-WAL-01 | 1d | TODO |
| TH-P2-WAL-03 | Enterprise Wallet Repository FindCreate | P2 | Enterprise | Finance / Repository | P1 | TH-P2-WAL-02 | 1d | TODO |
| TH-P2-WAL-04 | Enterprise Wallet TopUp Ledger | P2 | Enterprise | Finance / Service | P1 | TH-P2-WAL-03 | 1.5d | TODO |
| TH-P2-WAL-05 | Enterprise Wallet Reserve | P2 | Enterprise | Finance / Service | P1 | TH-P2-WAL-03 | 1.5d | TODO |
| TH-P2-WAL-06 | Enterprise Wallet Settle | P2 | Enterprise | Finance / Service | P1 | TH-P2-WAL-05 | 1.5d | TODO |
| TH-P2-WAL-07 | Enterprise Wallet Release | P2 | Enterprise | Finance / Service | P1 | TH-P2-WAL-05 | 1d | TODO |
| TH-P2-WAL-08 | Enterprise Wallet Transaction Listing | P2 | Enterprise | Backend / API | P1 | TH-P2-WAL-04, TH-P2-WAL-06 | 1d | TODO |
| TH-P2-WAL-09 | Enterprise Wallet Ledger Consistency Tests | P2 | Enterprise | Test / Finance | P1 | TH-P2-WAL-04, TH-P2-WAL-06, TH-P2-WAL-07 | 1.5d | TODO |
| TH-P2-GW-01 | Wallet Holder Resolver | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-WAL-03, TH-P2-INV-04 | 1d | TODO |
| TH-P2-GW-02 | Enterprise Status Fail-Closed | P2 | Enterprise | Gateway / Safety | P1 | TH-P2-GW-01 | 1d | TODO |
| TH-P2-GW-03 | Chat Endpoint Enterprise Reserve Integration | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-GW-02, TH-P2-WAL-05 | 1.5d | TODO |
| TH-P2-GW-04 | Responses And Claude Enterprise Reserve Integration | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-GW-03 | 1.5d | TODO |
| TH-P2-GW-05 | Embeddings And Images Enterprise Reserve Integration | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-GW-03 | 1.5d | TODO |
| TH-P2-GW-06 | Audio And Video Enterprise Reserve Integration | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-GW-03 | 1.5d | TODO |
| TH-P2-GW-07 | Enterprise Settle Release Endpoint Matrix | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-GW-03, TH-P2-GW-04, TH-P2-GW-05, TH-P2-GW-06, TH-P2-WAL-06, TH-P2-WAL-07 | 1.5d | TODO |
| TH-P2-LIM-01 | Monthly Limit Schema | P2 | Enterprise | Finance / Migration | P1 | TH-P2-INV-01 | 0.5d | TODO |
| TH-P2-LIM-02 | Monthly Limit Admin API | P2 | Enterprise | Backend / API | P1 | TH-P2-LIM-01, TH-P2-INV-04 | 1d | TODO |
| TH-P2-LIM-03 | Redis Monthly Counter | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-LIM-01 | 1.5d | TODO |
| TH-P2-LIM-04 | DB Usage Fallback | P2 | Enterprise | Gateway / Reliability | P1 | TH-P2-LIM-03 | 1.5d | TODO |
| TH-P2-LIM-05 | Limit Fail-Closed Policy | P2 | Enterprise | Gateway / Safety | P1 | TH-P2-LIM-04 | 1d | TODO |
| TH-P2-LIM-06 | Concurrent Limit Reservation | P2 | Enterprise | Test / Finance | P1 | TH-P2-LIM-05 | 1.5d | TODO |
| TH-P2-LIM-07 | Final Charge Counter Correction | P2 | Enterprise | Gateway / Finance | P1 | TH-P2-LIM-06, TH-P2-GW-07 | 1.5d | TODO |
| TH-P2-UI-01 | Enterprise Usage Query API | P2 | Enterprise | Backend / Reporting | P2 | TH-P2-GW-07 | 1.5d | TODO |
| TH-P2-UI-02 | Enterprise Statement API | P2 | Enterprise | Backend / Reporting | P2 | TH-P2-UI-01 | 1.5d | TODO |
| TH-P2-UI-03 | Enterprise Wallet View API | P2 | Enterprise | Backend / API | P2 | TH-P2-WAL-08 | 1d | TODO |
| TH-P2-UI-04 | Enterprise Center Route Shell | P2 | Enterprise | Frontend / Routing | P2 | TH-P2-04 | 1d | TODO |
| TH-P2-UI-05 | Enterprise Members UI | P2 | Enterprise | Frontend / UI | P2 | TH-P2-INV-03, TH-P2-INV-05, TH-P2-INV-06, TH-P2-LIM-02 | 1.5d | TODO |
| TH-P2-UI-06 | Enterprise Wallet UI | P2 | Enterprise | Frontend / UI | P2 | TH-P2-UI-03 | 1d | TODO |
| TH-P2-UI-07 | Enterprise Usage Statement UI | P2 | Enterprise | Frontend / UI | P2 | TH-P2-UI-01, TH-P2-UI-02 | 1.5d | TODO |
| TH-P2-UI-08 | Enterprise Access E2E Matrix | P2 | Enterprise | Test / E2E | P2 | TH-P2-UI-05, TH-P2-UI-06, TH-P2-UI-07 | 1.5d | TODO |
| TH-P3-01 | Permission Catalog Definition | P3 | RBAC | Backend / Authorization | P1 | TH-P05-09 | 1d | TODO |
| TH-P3-02 | RBAC Migration And Seed Roles | P3 | RBAC | Backend / Migration | P1 | TH-P3-01 | 1.5d | TODO |
| TH-P3-03 | Role Repository | P3 | RBAC | Backend / Repository | P1 | TH-P3-02 | 1d | TODO |
| TH-P3-04 | Role List Create API | P3 | RBAC | Backend / Admin API | P1 | TH-P3-03 | 1d | TODO |
| TH-P3-05 | Role Detail Update API | P3 | RBAC | Backend / Admin API | P1 | TH-P3-04 | 1.5d | TODO |
| TH-P3-06 | Role Delete Guard | P3 | RBAC | Backend / Admin API | P1 | TH-P3-04 | 1d | TODO |
| TH-P3-07 | User Role Assignment Guard | P3 | RBAC | Backend / Admin API | P1 | TH-P3-03 | 1.5d | TODO |
| TH-P3-08 | Current User Permissions API | P3 | RBAC | Backend / API | P1 | TH-P3-07 | 1d | TODO |
| TH-P3-09 | RequirePerm Middleware | P3 | RBAC | Backend / Authorization | P1 | TH-P3-08 | 1.5d | TODO |
| TH-P3-10 | Admin Route Permission Matrix | P3 | RBAC | Backend / Authorization | P1 | TH-P3-09 | 1.5d | TODO |
| TH-P3-11 | Authorization Failure Audit Event | P3 | RBAC | Audit / Security | P1 | TH-P3-09 | 1d | TODO |
| TH-P3-12 | Frontend Permission Hydration | P3 | RBAC | Frontend / State | P2 | TH-P3-08 | 1d | TODO |
| TH-P3-13 | Admin Navigation Permission Filtering | P3 | RBAC | Frontend / Navigation | P2 | TH-P3-12 | 1d | TODO |
| TH-P3-14 | Admin Action Permission Gating | P3 | RBAC | Frontend / UI | P2 | TH-P3-12 | 1.5d | TODO |
| TH-P3-15 | Role Management UI | P3 | RBAC | Frontend / UI | P2 | TH-P3-05, TH-P3-06, TH-P3-12 | 1.5d | TODO |
| TH-P4-01 | Nuxt Runtime Config | P4 | Landing Page | Frontend / Config | P2 | TH-P05-09 | 0.5d | TODO |
| TH-P4-02 | Nitro Public Proxy Base Client | P4 | Landing Page | Frontend / Nitro | P2 | TH-P4-01 | 1d | TODO |
| TH-P4-03 | Proxy Cache And Timeout | P4 | Landing Page | Frontend / Reliability | P2 | TH-P4-02 | 1d | TODO |
| TH-P4-04 | Proxy Fallback Snapshot Metadata | P4 | Landing Page | Frontend / Data | P2 | TH-P4-03 | 1d | TODO |
| TH-P4-05 | Pricing Payload Mapper | P4 | Landing Page | Frontend / Data Mapping | P2 | TH-P4-04 | 1d | TODO |
| TH-P4-06 | Pricing Page Async Data | P4 | Landing Page | Frontend / Vue | P2 | TH-P4-05 | 1d | TODO |
| TH-P4-07 | Pricing Modal Live Data Regression | P4 | Landing Page | Frontend / Vue | P2 | TH-P4-06 | 1d | TODO |
| TH-P4-08 | Home Site Stats Mapper | P4 | Landing Page | Frontend / Data Mapping | P2 | TH-P4-04 | 1d | TODO |
| TH-P4-09 | Home Rankings Featured Data | P4 | Landing Page | Frontend / Vue | P2 | TH-P4-08 | 1d | TODO |
| TH-P4-10 | Visual Parity Screenshot Baseline | P4 | Landing Page | Test / Visual Regression | P2 | TH-P4-06, TH-P4-09 | 1.5d | TODO |
| TH-P4-11 | Auth Redirect URL Builder | P4 | Landing Page | Frontend / Routing | P2 | TH-P4-01 | 1d | TODO |
| TH-P4-12 | Nuxt Auth Prototype Route Retirement | P4 | Landing Page | Frontend / Routing | P2 | TH-P4-11 | 1d | TODO |
| TH-P5-RVW-01 | Reconciliation Review Item Schema | P5 | Production Hardening | Finance / Migration | P1 | TH-P05-09 | 1.5d | TODO |
| TH-P5-RVW-02 | Undercharge Diff Review Item Producer | P5 | Production Hardening | Worker / Finance | P1 | TH-P5-RVW-01 | 1.5d | TODO |
| TH-P5-RVW-03 | Review Item Admin List Detail API | P5 | Production Hardening | Backend / Admin API | P1 | TH-P5-RVW-02 | 1d | TODO |
| TH-P5-RVW-04 | Manual Adjustment Command API | P5 | Production Hardening | Backend / Finance | P1 | TH-P5-RVW-03 | 1.5d | TODO |
| TH-P5-RVW-05 | Adjustment Ledger Idempotency Guard | P5 | Production Hardening | Finance / Ledger | P1 | TH-P5-RVW-04 | 1.5d | TODO |
| TH-P5-RVW-06 | Review Close Waive Actions | P5 | Production Hardening | Backend / Finance | P1 | TH-P5-RVW-03 | 1d | TODO |
| TH-P5-RVW-07 | Evidence Immutability Regression | P5 | Production Hardening | Test / Finance | P1 | TH-P5-RVW-02, TH-P5-RVW-05 | 1d | TODO |
| TH-P5-TEL-01 | Inference Telemetry Schema | P5 | Production Hardening | Observability / Migration | P2 | TH-P05-09 | 1.5d | TODO |
| TH-P5-TEL-02 | Request Timing Instrumentation | P5 | Production Hardening | Observability / Gateway | P2 | TH-P5-TEL-01 | 1.5d | TODO |
| TH-P5-TEL-03 | TTFT Collection | P5 | Production Hardening | Observability / Gateway | P2 | TH-P5-TEL-02 | 1.5d | TODO |
| TH-P5-TEL-04 | Token Throughput Calculation | P5 | Production Hardening | Observability / Gateway | P2 | TH-P5-TEL-03 | 1d | TODO |
| TH-P5-TEL-05 | Provider Channel Endpoint Attribution | P5 | Production Hardening | Observability / Gateway | P2 | TH-P5-TEL-01 | 1d | TODO |
| TH-P5-TEL-06 | Cost Gross Margin Attribution | P5 | Production Hardening | Observability / Finance | P2 | TH-P5-TEL-01 | 1.5d | TODO |
| TH-P5-TEL-07 | Error Taxonomy | P5 | Production Hardening | Observability / Gateway | P2 | TH-P5-TEL-01 | 1d | TODO |
| TH-P5-TEL-08 | Telemetry Persistence | P5 | Production Hardening | Observability / Repository | P2 | TH-P5-TEL-02, TH-P5-TEL-05, TH-P5-TEL-06, TH-P5-TEL-07 | 1.5d | TODO |
| TH-P5-TEL-09 | Telemetry Regression Tests | P5 | Production Hardening | Test / Observability | P2 | TH-P5-TEL-03, TH-P5-TEL-04, TH-P5-TEL-08 | 1.5d | TODO |
| TH-P5-MET-01 | Prometheus Metrics Endpoint Wiring | P5 | Production Hardening | Observability | P2 | TH-P05-04 | 1d | TODO |
| TH-P5-MET-02 | Gateway System Metrics Labels | P5 | Production Hardening | Observability / Gateway | P2 | TH-P5-MET-01 | 1d | TODO |
| TH-P5-MET-03 | Payment Worker Reconciliation Metrics | P5 | Production Hardening | Observability / Worker | P2 | TH-P5-MET-01, TH-P1-CW-08, TH-P1-RC-03 | 1.5d | TODO |
| TH-P5-SEC-01 | Sensitive Log Redaction Tests | P5 | Production Hardening | Security / Test | P2 | TH-P05-09 | 1.5d | TODO |
| TH-P5-SEC-02 | JWT Rotation Drill | P5 | Production Hardening | Security / Operations | P2 | TH-P5-SEC-01 | 1d | TODO |
| TH-P5-SEC-03 | Encryption Key Rotation Design | P5 | Production Hardening | Security / Design | P2 | TH-P5-SEC-01 | 1d | TODO |
| TH-P5-LOAD-01 | Load Failure Harness Scenarios | P5 | Production Hardening | Test Harness / Reliability | P2 | TH-P05-09, TH-P5-MET-01 | 1.5d | TODO |

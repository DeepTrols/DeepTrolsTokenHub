# P5 Production Hardening Sprint Tasks

## Epic Code Audit

Existing:

- Health endpoints exist and deployment docs include basic deployment and backup commands.
- Worker has Redis lease infrastructure.
- L0/L1/L2/L3 reconciliation code detects differences.
- Documentation explicitly states: reconciliation tasks can find differences, but cannot directly modify wallets; money actions must enter review.

Partial:

- Metrics and alerts are incomplete outside basic health checks.
- Reconciliation detects differences, but review item and manual adjustment workflow are missing.
- Backup instructions exist, but restore drill is covered in P0.5 and should not be deferred here.
- Gateway tests cover some timing and billing behavior, but request-level telemetry persistence for future Smart Routing is missing.

Missing:

- Reconciliation review item schema/API/actions and adjustment idempotency.
- Inference telemetry schema, request timing, TTFT, throughput, attribution, cost/margin, error taxonomy, persistence, and regression tests.
- Prometheus endpoint wiring, label discipline, payment/worker metrics, sensitive log tests, secret rotation drills, and load/failure harness.

Risks:

- Any reconciliation path that directly modifies wallets can become a funds incident.
- Prometheus aggregates cannot replace request/endpoint-level telemetry needed for future Smart Routing.
- Load and failure tests without sensitive log checks can leak keys or prompts during incidents.

## Split Rule

Every Production Hardening task is sized at 0.5d-2d and can be independently developed, reviewed, tested, accepted, and rolled back. Backup baseline, restore drill, and clean deployment are owned by P0.5, not buried in P5.

## Reconciliation Review And Adjustment Workflow

### TH-P5-RVW-01

Task ID: TH-P5-RVW-01
Title: Reconciliation Review Item Schema
Phase: P5
Epic: Production Hardening
Type: Finance / Migration
Priority: P1
Dependencies: TH-P05-09
Estimate: 1.5d

Objective: Add schema for reconciliation differences and review items without wallet mutation.

Current State: Reconciliation differences can be detected, but no dedicated review item workflow exists.

Scope: Create `reconciliation_diff` and `review_item` schema with request id, evidence, expected amount, actual amount, difference, reason, status, assignee, and timestamps.

Out of Scope: Worker production logic, admin UI, and wallet adjustment execution.

Implementation Notes: Preserve immutable evidence references and model status transitions separately from wallet ledger.

Acceptance Criteria:

- AC-01: Given migration runs on clean DB, diff and review item tables are created with request id, evidence, expected amount, actual amount, difference, reason, and status columns.
- AC-02: Given rollback runs, new review workflow tables are removed without touching usage or wallet ledger tables.
- AC-03: Given duplicate diff idempotency key is inserted, DB constraint rejects the duplicate.

Test Requirements: Integration migration up/down tests; regression existing reconciliation tests; failure injection for migration failure.

Observability Requirements: No runtime metric required.

Audit Requirements: Schema must allow audit linkage to review item and operator actions.

Migration / Rollback Requirements: Migration and rollback are required and verified.

Documentation Requirements: Document review item statuses and required fields.

Risks: Missing immutable evidence fields can make finance review unverifiable.

Definition of Done: Global DoD applies.

### TH-P5-RVW-02

Task ID: TH-P5-RVW-02
Title: Undercharge Diff Review Item Producer
Phase: P5
Epic: Production Hardening
Type: Worker / Finance
Priority: P1
Dependencies: TH-P5-RVW-01
Estimate: 1.5d

Objective: Convert undercharge reconciliation findings into review items without changing wallet balances.

Current State: Reconciliation can classify differences, but undercharge review item production is missing.

Scope: Generate reconciliation diff and review item records for undercharge findings.

Out of Scope: Wallet spend, wallet adjust, wallet top-up, and manual adjustment approval.

Implementation Notes: Reconciliation worker must treat wallet services as forbidden dependencies for this path.

Acceptance Criteria:

- AC-01: Given reconciliation worker handles an undercharge finding, it does not call Wallet Spend, Wallet Adjust, or Wallet TopUp.
- AC-02: Given an undercharge is detected, worker creates only reconciliation diff and review item records.
- AC-03: Given review item is created, it contains `request_id`, `evidence`, `expected_amount`, `actual_amount`, `difference`, `reason`, and `status`.
- AC-04: Given the same undercharge finding is processed twice, only one diff/review item pair exists.

Test Requirements: Unit producer decision tests; integration diff/review persistence; regression reconciliation classifier tests; duplicate consumption test.

Observability Requirements: Emit review item produced counter by diff type.

Audit Requirements: Write `reconciliation.review_item_created` with diff id and reason.

Migration / Rollback Requirements: No migration beyond TH-P5-RVW-01. Rollback disables producer and leaves existing review rows for manual handling.

Documentation Requirements: Document no-wallet-mutation worker rule.

Risks: A hidden wallet call in reconciliation would violate the product funds principle.

Definition of Done: Global DoD applies.

### TH-P5-RVW-03

Task ID: TH-P5-RVW-03
Title: Review Item Admin List Detail API
Phase: P5
Epic: Production Hardening
Type: Backend / Admin API
Priority: P1
Dependencies: TH-P5-RVW-02
Estimate: 1d

Objective: Expose reconciliation review items for finance/admin review.

Current State: No admin API exists for review item queue or details.

Scope: Add list, filter, and detail APIs with permission checks and immutable evidence display.

Out of Scope: Adjustment execution and UI.

Implementation Notes: Details must show evidence references without overwriting original usage or ledger.

Acceptance Criteria:

- AC-01: Given Super Admin lists review items, API returns status, request id, reason, difference, and created time.
- AC-02: Given admin without reconciliation review permission lists items, API returns 403.
- AC-03: Given item detail is requested, response includes original evidence references and does not mutate usage or ledger rows.

Test Requirements: Unit filter tests; integration unauthenticated/no-permission/allowed/Super Admin cases; regression audit route tests.

Observability Requirements: Emit review item read counter by result.

Audit Requirements: Audit forbidden review item access attempts.

Migration / Rollback Requirements: No migration. Rollback disables review APIs.

Documentation Requirements: Document filters and response schema.

Risks: Evidence leakage can expose provider billing details to unauthorized users.

Definition of Done: Global DoD applies.

### TH-P5-RVW-04

Task ID: TH-P5-RVW-04
Title: Manual Adjustment Command API
Phase: P5
Epic: Production Hardening
Type: Backend / Finance
Priority: P1
Dependencies: TH-P5-RVW-03
Estimate: 1.5d

Objective: Create a manual, permission-gated adjustment command from a reviewed reconciliation diff.

Current State: No formal review-triggered adjustment path exists.

Scope: Add API action requiring operator, reason, source diff id, amount, before balance, after balance, and explicit review decision.

Out of Scope: Worker-triggered funds changes and UI.

Implementation Notes: The API calls wallet adjustment only after manual review action passes permission and status checks.

Acceptance Criteria:

- AC-01: Given a finance operator approves adjustment for an open review item, API creates an adjustment command and records operator, reason, source diff id, before balance, amount, and after balance.
- AC-02: Given reconciliation worker has only produced the review item, no wallet balance changes before this manual API action.
- AC-03: Given admin without adjustment permission calls the action, API returns 403 and wallet balance remains unchanged.
- AC-04: Given review item status is closed, adjustment action returns 409 and wallet balance remains unchanged.

Test Requirements: Unit status and permission matrix; integration wallet adjustment command path; failure injection for wallet service error.

Observability Requirements: Emit manual adjustment attempt counter by result.

Audit Requirements: Write `reconciliation.adjustment_requested` with operator, source diff id, amount, and reason.

Migration / Rollback Requirements: No migration. Rollback disables adjustment command route.

Documentation Requirements: Document manual review-to-adjustment flow.

Risks: Allowing adjustment from non-review context can bypass finance controls.

Definition of Done: Global DoD applies.

### TH-P5-RVW-05

Task ID: TH-P5-RVW-05
Title: Adjustment Ledger Idempotency Guard
Phase: P5
Epic: Production Hardening
Type: Finance / Ledger
Priority: P1
Dependencies: TH-P5-RVW-04
Estimate: 1.5d

Objective: Ensure one approved reconciliation diff can create at most one wallet adjustment.

Current State: Manual adjustment path is planned, but idempotency guard is separate.

Scope: Add unique `source_diff_id` guard, ledger linkage, retry behavior, and balance consistency checks.

Out of Scope: Review list UI and worker producer.

Implementation Notes: Adjustment must be a ledgered wallet service call, never direct balance mutation.

Acceptance Criteria:

- AC-01: Given the same `source_diff_id` adjustment is submitted twice, exactly one wallet adjustment ledger entry exists.
- AC-02: Given adjustment succeeds, ledger entry records operator, reason, source diff id, before balance, amount, and after balance.
- AC-03: Given ledger write fails after balance calculation, transaction rolls back and wallet balance remains unchanged.
- AC-04: Given wallet balance is insufficient for a debit adjustment, API returns 409 and no ledger entry is created.

Test Requirements: Unit idempotency tests; integration ledger/balance tests; concurrency and failure injection tests.

Observability Requirements: Emit duplicate adjustment rejection metric and ledger failure metric.

Audit Requirements: Write `reconciliation.adjustment_committed` on success and audit rejected duplicates.

Migration / Rollback Requirements: Migration only if unique guard requires schema change; rollback drops the guard after confirming no duplicate rows.

Documentation Requirements: Document source diff idempotency rule.

Risks: Duplicate adjustment can double-charge or double-credit customer wallets.

Definition of Done: Global DoD applies.

### TH-P5-RVW-06

Task ID: TH-P5-RVW-06
Title: Review Close Waive Actions
Phase: P5
Epic: Production Hardening
Type: Backend / Finance
Priority: P1
Dependencies: TH-P5-RVW-03
Estimate: 1d

Objective: Allow finance reviewers to close or waive reconciliation review items without funds movement.

Current State: Review item workflow is planned; non-adjustment decisions are missing.

Scope: Add close and waive actions with reason, operator, status transition validation, and audit.

Out of Scope: Wallet adjustment and UI.

Implementation Notes: Closing or waiving a review item must not call wallet services.

Acceptance Criteria:

- AC-01: Given an open review item is waived by authorized reviewer, status becomes `waived` and wallet balance remains unchanged.
- AC-02: Given an open review item is closed as non-actionable, status becomes `closed` and original evidence remains unchanged.
- AC-03: Given same close/waive request is repeated, status remains final and no duplicate audit success event is written.

Test Requirements: Unit status transition tests; integration permission and no-wallet-call tests; duplicate request test.

Observability Requirements: Emit review close/waive counter by result.

Audit Requirements: Write `reconciliation.review_item_closed` or `reconciliation.review_item_waived` with operator and reason.

Migration / Rollback Requirements: No migration. Rollback disables close/waive routes.

Documentation Requirements: Document final statuses.

Risks: Missing close path leaves non-actionable differences stuck forever.

Definition of Done: Global DoD applies.

### TH-P5-RVW-07

Task ID: TH-P5-RVW-07
Title: Evidence Immutability Regression
Phase: P5
Epic: Production Hardening
Type: Test / Finance
Priority: P1
Dependencies: TH-P5-RVW-02, TH-P5-RVW-05
Estimate: 1d

Objective: Prove reconciliation review and adjustment actions never overwrite original usage, ledger, or evidence.

Current State: Evidence immutability is a documented principle but not tested for the new workflow.

Scope: Add regression tests covering review item creation, adjustment, close, waive, and repeated processing.

Out of Scope: UI and provider bill import.

Implementation Notes: Tests compare row snapshots before and after each action.

Acceptance Criteria:

- AC-01: Given review item is created, original usage row remains byte-for-byte unchanged for tested fields.
- AC-02: Given manual adjustment is committed, original wallet ledger rows remain unchanged and a new linked adjustment row is appended.
- AC-03: Given close or waive action completes, original evidence payload remains unchanged.
- AC-04: Given repeated reconciliation processing occurs, evidence rows are not overwritten.

Test Requirements: Regression tests required; integration DB snapshot tests; duplicate consumption test.

Observability Requirements: No runtime metric required.

Audit Requirements: Tests assert every review action has an audit event.

Migration / Rollback Requirements: No migration. Rollback removes tests only.

Documentation Requirements: Document evidence immutability rule.

Risks: Evidence overwrites make later finance review impossible.

Definition of Done: Global DoD applies.

## Inference Telemetry For Future Smart Routing

### TH-P5-TEL-01

Task ID: TH-P5-TEL-01
Title: Inference Telemetry Schema
Phase: P5
Epic: Production Hardening
Type: Observability / Migration
Priority: P2
Dependencies: TH-P05-09
Estimate: 1.5d

Objective: Add request-level telemetry schema for future Smart Routing analysis.

Current State: Aggregate logs and usage data exist, but no dedicated endpoint-level telemetry fact table exists.

Scope: Store model, provider, channel, endpoint, request type, latency, TTFT, duration, tokens, status, error type, retry count, provider cost, customer cost, and gross margin.

Out of Scope: Smart Router implementation and dashboard UI.

Implementation Notes: Treat telemetry as append-only facts linked by request id.

Acceptance Criteria:

- AC-01: Given migration runs, telemetry table includes columns for model, provider, channel, endpoint, request type, timing, token, status, error, retry, and cost/margin fields.
- AC-02: Given rollback runs, telemetry schema is removed without changing usage or wallet ledger tables.
- AC-03: Given duplicate request id telemetry insert is attempted, idempotency behavior is deterministic and documented.

Test Requirements: Integration migration up/down tests; regression usage and wallet migration tests.

Observability Requirements: No metric required for schema-only slice.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: Migration and rollback are required and verified.

Documentation Requirements: Document telemetry field dictionary.

Risks: Missing request-level dimensions can block future routing decisions.

Definition of Done: Global DoD applies.

### TH-P5-TEL-02

Task ID: TH-P5-TEL-02
Title: Request Timing Instrumentation
Phase: P5
Epic: Production Hardening
Type: Observability / Gateway
Priority: P2
Dependencies: TH-P5-TEL-01
Estimate: 1.5d

Objective: Capture latency and duration timing for gateway inference requests.

Current State: Request logs exist, but timing is not persisted as telemetry facts.

Scope: Instrument request start, upstream dispatch, response completion, and total duration for non-streaming and streaming paths.

Out of Scope: TTFT and token throughput calculation.

Implementation Notes: Timing capture must not change response behavior.

Acceptance Criteria:

- AC-01: Given a successful non-streaming request, telemetry contains positive latency and duration values.
- AC-02: Given a streaming request completes, telemetry contains total duration from request start to stream close.
- AC-03: Given upstream call fails, telemetry still records duration and failure status.

Test Requirements: Unit timing helper tests; integration gateway success/failure timing tests; regression response body tests.

Observability Requirements: Emit telemetry write attempt metric by result.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes instrumentation hooks.

Documentation Requirements: Document timing definitions.

Risks: Instrumentation that wraps response streams incorrectly can break streaming.

Definition of Done: Global DoD applies.

### TH-P5-TEL-03

Task ID: TH-P5-TEL-03
Title: TTFT Collection
Phase: P5
Epic: Production Hardening
Type: Observability / Gateway
Priority: P2
Dependencies: TH-P5-TEL-02
Estimate: 1.5d

Objective: Capture time to first token for streaming inference requests.

Current State: Streaming gateway tests exist, but TTFT is not recorded.

Scope: Record first meaningful upstream token/chunk timestamp and persist TTFT in telemetry.

Out of Scope: Token throughput and UI.

Implementation Notes: Handle empty role-only chunks separately from content-bearing chunks.

Acceptance Criteria:

- AC-01: Given a streaming response emits first content chunk after 250ms, telemetry TTFT is between 200ms and 400ms in controlled test.
- AC-02: Given a stream closes before content chunk, telemetry TTFT is null and status records failure or empty stream reason.
- AC-03: Given non-streaming request completes, telemetry TTFT is null.

Test Requirements: Unit chunk classifier tests; integration streaming TTFT tests; failure injection for early stream close.

Observability Requirements: Emit TTFT missing counter by reason.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes TTFT hook.

Documentation Requirements: Document TTFT definition.

Risks: Counting role-only chunks as first token can corrupt routing data.

Definition of Done: Global DoD applies.

### TH-P5-TEL-04

Task ID: TH-P5-TEL-04
Title: Token Throughput Calculation
Phase: P5
Epic: Production Hardening
Type: Observability / Gateway
Priority: P2
Dependencies: TH-P5-TEL-03
Estimate: 1d

Objective: Calculate tokens per second from output tokens and request duration.

Current State: Usage logs may store token counts, but throughput is not persisted.

Scope: Compute `tokens_per_second` for successful requests with output tokens and positive duration.

Out of Scope: Token counting algorithm changes.

Implementation Notes: Leave throughput null when output tokens or duration are unavailable.

Acceptance Criteria:

- AC-01: Given output tokens are 100 and duration is 5 seconds, telemetry records `tokens_per_second=20`.
- AC-02: Given output tokens are missing, telemetry throughput is null.
- AC-03: Given duration is zero or negative due to bad clock input, telemetry throughput is null and warning metric increments.

Test Requirements: Unit formula tests; integration telemetry persistence check; regression usage token parsing tests.

Observability Requirements: Emit invalid throughput input counter.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes derived field calculation.

Documentation Requirements: Document formula and null cases.

Risks: Incorrect throughput data can mislead future routing.

Definition of Done: Global DoD applies.

### TH-P5-TEL-05

Task ID: TH-P5-TEL-05
Title: Provider Channel Endpoint Attribution
Phase: P5
Epic: Production Hardening
Type: Observability / Gateway
Priority: P2
Dependencies: TH-P5-TEL-01
Estimate: 1d

Objective: Persist provider, channel, endpoint, model, and request type attribution for each inference request.

Current State: Routing data exists during request handling, but historical telemetry attribution is missing.

Scope: Capture selected provider, selected channel, endpoint path/category, model, and request type.

Out of Scope: Smart Routing score calculation.

Implementation Notes: Attribution must be written for success and failure outcomes after routing decision.

Acceptance Criteria:

- AC-01: Given a routed chat request succeeds, telemetry includes model, provider, channel, endpoint, and request type.
- AC-02: Given upstream request fails after channel selection, telemetry still includes selected provider and channel.
- AC-03: Given routing fails before channel selection, telemetry records endpoint and model with null provider/channel and failure status.

Test Requirements: Unit attribution mapper tests; integration gateway success and route-failure tests.

Observability Requirements: Emit missing attribution counter by field.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes attribution hook.

Documentation Requirements: Document attribution source per field.

Risks: Missing failure attribution hides unhealthy providers.

Definition of Done: Global DoD applies.

### TH-P5-TEL-06

Task ID: TH-P5-TEL-06
Title: Cost Gross Margin Attribution
Phase: P5
Epic: Production Hardening
Type: Observability / Finance
Priority: P2
Dependencies: TH-P5-TEL-01
Estimate: 1.5d

Objective: Persist provider cost, customer cost, and gross margin for each inference telemetry record.

Current State: Usage and billing records hold cost information, but telemetry lacks margin facts.

Scope: Map provider cost, customer cost, currency, and gross margin from billing results to telemetry.

Out of Scope: Pricing formula changes and Smart Routing.

Implementation Notes: Costs must match ledger/usage facts and not recompute with current catalog prices after the request.

Acceptance Criteria:

- AC-01: Given provider cost is 3.00 and customer cost is 5.00, telemetry gross margin is 2.00.
- AC-02: Given billing result is unavailable due to gateway failure before upstream call, cost fields are null and status records failure.
- AC-03: Given usage ledger later differs from telemetry cost, reconciliation can identify request id without rewriting telemetry.

Test Requirements: Unit cost mapper tests; integration billing-to-telemetry tests; regression billing invariant tests.

Observability Requirements: Emit cost attribution missing counter by reason.

Audit Requirements: No audit event required; telemetry must not contain API keys or prompts.

Migration / Rollback Requirements: No migration. Rollback removes cost attribution hook.

Documentation Requirements: Document cost source and gross margin formula.

Risks: Repricing historical requests with current prices corrupts routing economics.

Definition of Done: Global DoD applies.

### TH-P5-TEL-07

Task ID: TH-P5-TEL-07
Title: Error Taxonomy
Phase: P5
Epic: Production Hardening
Type: Observability / Gateway
Priority: P2
Dependencies: TH-P5-TEL-01
Estimate: 1d

Objective: Define and persist normalized error types for inference telemetry.

Current State: Errors are logged but not normalized for historical analysis.

Scope: Map auth, quota, wallet, routing, provider timeout, provider 5xx, parse failure, client cancel, and unknown errors.

Out of Scope: Alert rules and UI.

Implementation Notes: Preserve raw error internally where appropriate but store only safe normalized taxonomy in telemetry.

Acceptance Criteria:

- AC-01: Given provider timeout occurs, telemetry `error_type` equals `provider_timeout`.
- AC-02: Given wallet reserve fails for insufficient balance, telemetry `error_type` equals `wallet_insufficient_balance`.
- AC-03: Given client cancels streaming request, telemetry `error_type` equals `client_cancelled`.
- AC-04: Given error contains API key text, telemetry stores normalized type only and does not store the secret.

Test Requirements: Unit taxonomy mapper tests; integration gateway error cases; failure injection for timeout and client cancel.

Observability Requirements: Emit error type counter.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes taxonomy mapper.

Documentation Requirements: Document error taxonomy.

Risks: High-cardinality raw errors will make telemetry noisy and unsafe.

Definition of Done: Global DoD applies.

### TH-P5-TEL-08

Task ID: TH-P5-TEL-08
Title: Telemetry Persistence
Phase: P5
Epic: Production Hardening
Type: Observability / Repository
Priority: P2
Dependencies: TH-P5-TEL-02, TH-P5-TEL-05, TH-P5-TEL-06, TH-P5-TEL-07
Estimate: 1.5d

Objective: Persist telemetry facts reliably without blocking the gateway response path.

Current State: Schema and mappers are split across previous telemetry tasks; persistence orchestration is missing.

Scope: Add repository insert/upsert behavior, async or bounded write policy, and failure logging.

Out of Scope: Dashboard and Smart Routing queries.

Implementation Notes: Telemetry write failure must not double-charge or change request response.

Acceptance Criteria:

- AC-01: Given inference request succeeds, one telemetry row is persisted with timing, attribution, status, token, and cost fields.
- AC-02: Given telemetry DB insert fails after gateway response is ready, response body and billing ledger remain unchanged.
- AC-03: Given same request id is written twice, persistence follows documented idempotency behavior and does not create conflicting facts.

Test Requirements: Unit repository tests; integration success/failure persistence tests; failure injection for telemetry DB outage.

Observability Requirements: Emit telemetry write success/failure counters and queue lag metric if async path is used.

Audit Requirements: Telemetry rows must exclude prompts, API keys, and tokens.

Migration / Rollback Requirements: No migration beyond TH-P5-TEL-01. Rollback disables telemetry writer.

Documentation Requirements: Document write failure policy.

Risks: Blocking response on telemetry can amplify DB incidents.

Definition of Done: Global DoD applies.

### TH-P5-TEL-09

Task ID: TH-P5-TEL-09
Title: Telemetry Regression Tests
Phase: P5
Epic: Production Hardening
Type: Test / Observability
Priority: P2
Dependencies: TH-P5-TEL-03, TH-P5-TEL-04, TH-P5-TEL-08
Estimate: 1.5d

Objective: Build regression tests for complete telemetry behavior across success, failure, streaming, and cost cases.

Current State: Individual telemetry tasks define behavior; cross-path regression suite is missing.

Scope: Cover model, provider, channel, endpoint, request type, latency, TTFT, duration, token counts, throughput, status, error type, retry count, costs, and margin.

Out of Scope: Smart Router tests and dashboard tests.

Implementation Notes: Tests must assert sensitive data is absent.

Acceptance Criteria:

- AC-01: Given a successful streaming request, telemetry row contains model, provider, channel, endpoint, TTFT, duration, output tokens, throughput, status, costs, and margin.
- AC-02: Given provider timeout, telemetry row contains provider/channel attribution, failure status, `provider_timeout`, retry count, and no prompt body.
- AC-03: Given wallet insufficient balance occurs before upstream, telemetry row records endpoint/model, wallet error type, null provider cost, and no upstream channel if none selected.

Test Requirements: Integration regression suite; failure injection for timeout and DB write failure; concurrency test for duplicate request id.

Observability Requirements: Test verifies telemetry write metrics.

Audit Requirements: Test verifies no secrets, tokens, or prompts are persisted in telemetry.

Migration / Rollback Requirements: No migration. Rollback removes tests only.

Documentation Requirements: Document regression fixture set.

Risks: Partial telemetry coverage can make future Smart Routing train on biased data.

Definition of Done: Global DoD applies.

## Metrics Security And Load Hardening

### TH-P5-MET-01

Task ID: TH-P5-MET-01
Title: Prometheus Metrics Endpoint Wiring
Phase: P5
Epic: Production Hardening
Type: Observability
Priority: P2
Dependencies: TH-P05-04
Estimate: 1d

Objective: Expose Prometheus-compatible metrics endpoint for system and application counters.

Current State: Health endpoints exist, but Prometheus endpoint wiring is incomplete.

Scope: Add metrics endpoint, registry setup, request count, latency, and error counters.

Out of Scope: Alert rules and request-level telemetry persistence.

Implementation Notes: Protect metrics endpoint according to deployment topology.

Acceptance Criteria:

- AC-01: Given service is running, metrics endpoint returns Prometheus text format.
- AC-02: Given one successful API request and one failing API request occur, metrics output includes updated request and error counters.
- AC-03: Given metrics endpoint is disabled by config, route returns 404 or configured denied response.

Test Requirements: Unit metric registration tests; integration metrics endpoint test; regression health endpoint tests.

Observability Requirements: This task creates the metrics endpoint.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback disables metrics route.

Documentation Requirements: Update deployment metrics section.

Risks: Public metrics can leak route names or internal topology.

Definition of Done: Global DoD applies.

### TH-P5-MET-02

Task ID: TH-P5-MET-02
Title: Gateway System Metrics Labels
Phase: P5
Epic: Production Hardening
Type: Observability / Gateway
Priority: P2
Dependencies: TH-P5-MET-01
Estimate: 1d

Objective: Add low-cardinality gateway metrics labels for model, provider, endpoint, and status.

Current State: Gateway lacks complete metrics label discipline.

Scope: Record request count, latency buckets, upstream error count, and billing outcome count with bounded labels.

Out of Scope: Request-level telemetry table.

Implementation Notes: Do not label by user id, API key, raw URL, prompt, or request id.

Acceptance Criteria:

- AC-01: Given gateway request succeeds, metrics include endpoint, model, provider, and status labels.
- AC-02: Given provider fails, metrics include provider and normalized failure status.
- AC-03: Given request id or user id exists, metrics labels do not contain either value.

Test Requirements: Unit label sanitizer tests; integration gateway metrics tests; regression request handling tests.

Observability Requirements: This task defines gateway metrics labels.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes metric collectors.

Documentation Requirements: Document allowed labels.

Risks: High-cardinality labels can make Prometheus unstable.

Definition of Done: Global DoD applies.

### TH-P5-MET-03

Task ID: TH-P5-MET-03
Title: Payment Worker Reconciliation Metrics
Phase: P5
Epic: Production Hardening
Type: Observability / Worker
Priority: P2
Dependencies: TH-P5-MET-01, TH-P1-CW-08, TH-P1-RC-03
Estimate: 1.5d

Objective: Add metrics for payment compensation and reconciliation workers.

Current State: Worker lease observability is in P0.5; payment and reconciliation domain metrics remain separate.

Scope: Track scanned orders, query results, retries, max retries, duplicate callbacks, diff counts, and review item production.

Out of Scope: Worker implementation and review APIs.

Implementation Notes: Keep provider labels bounded to known channel ids.

Acceptance Criteria:

- AC-01: Given compensation worker scans pending orders, metrics increment scanned count and result count.
- AC-02: Given reconciliation creates an undercharge review item, metrics increment diff and review item counters.
- AC-03: Given worker reaches max retry, metrics increment max retry counter.

Test Requirements: Unit metric hook tests; integration worker metric output tests; regression lease metrics tests.

Observability Requirements: This task defines payment/reconciliation worker metrics.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes metric hooks.

Documentation Requirements: Document worker metric names.

Risks: Missing worker metrics hides failed compensation and reconciliation backlogs.

Definition of Done: Global DoD applies.

### TH-P5-SEC-01

Task ID: TH-P5-SEC-01
Title: Sensitive Log Redaction Tests
Phase: P5
Epic: Production Hardening
Type: Security / Test
Priority: P2
Dependencies: TH-P05-09
Estimate: 1.5d

Objective: Verify logs never contain secrets, API keys, tokens, payment signatures, or sensitive payment payloads.

Current State: Logging exists, but complete sensitive-data regression tests are missing.

Scope: Add tests for gateway, payment callback, admin auth, worker, and telemetry error paths.

Out of Scope: Secret rotation and SIEM export.

Implementation Notes: Include positive fixtures that would fail if raw secrets are logged.

Acceptance Criteria:

- AC-01: Given gateway request includes API key and prompt, captured logs contain neither value.
- AC-02: Given payment callback includes signature and provider payload, captured logs contain neither raw signature nor sensitive payment payload.
- AC-03: Given worker error includes token-like string, captured logs contain redacted placeholder only.

Test Requirements: Unit log sanitizer tests; integration captured log tests; regression gateway/payment/worker error tests.

Observability Requirements: Emit redaction test failure only in test output; no runtime metric required.

Audit Requirements: Audit payload tests must also exclude secrets.

Migration / Rollback Requirements: No migration. Rollback removes tests only.

Documentation Requirements: Document sensitive log denylist.

Risks: Incident logs can leak production credentials or payment data.

Definition of Done: Global DoD applies.

### TH-P5-SEC-02

Task ID: TH-P5-SEC-02
Title: JWT Rotation Drill
Phase: P5
Epic: Production Hardening
Type: Security / Operations
Priority: P2
Dependencies: TH-P5-SEC-01
Estimate: 1d

Objective: Document and rehearse JWT secret rotation without breaking active deployment rollback.

Current State: JWT auth exists, but rotation drill is not documented and verified.

Scope: Define rotation steps, validation, rollback, session impact, and log redaction checks.

Out of Scope: Key management service integration.

Implementation Notes: Use non-production environment for drill.

Acceptance Criteria:

- AC-01: Given old and new JWT secret sequence is followed in staging, new logins receive tokens signed by new secret.
- AC-02: Given rollback step is executed, service accepts the documented rollback state.
- AC-03: Given logs are captured during drill, secret values do not appear in logs.

Test Requirements: Integration auth smoke in staging; manual rotation drill; regression login/session tests.

Observability Requirements: Record auth failure rate during drill.

Audit Requirements: Audit operator change event for secret rotation config.

Migration / Rollback Requirements: Runbook must include rollback sequence.

Documentation Requirements: Update `DEPLOYMENT.md` or security operations doc.

Risks: Rotation without rollback can force global logout or outage.

Definition of Done: Global DoD applies.

### TH-P5-SEC-03

Task ID: TH-P5-SEC-03
Title: Encryption Key Rotation Design
Phase: P5
Epic: Production Hardening
Type: Security / Design
Priority: P2
Dependencies: TH-P5-SEC-01
Estimate: 1d

Objective: Define rotation approach for encrypted credentials and provider secrets.

Current State: Secret storage exists, but key rotation design is not verified.

Scope: Document key id, dual-read, re-encryption, rollback, and failure handling plan.

Out of Scope: Rotation implementation.

Implementation Notes: Never print decrypted secrets in migration logs.

Acceptance Criteria:

- AC-01: Given design is reviewed, it includes dual-read period, re-encryption process, and rollback decision points.
- AC-02: Given re-encryption fails for one secret, design states how to stop without deleting original encrypted value.
- AC-03: Given old key is retired, design states verification query proving no active rows depend on old key.

Test Requirements: Manual security review; integration feasibility check against secret tables; regression checklist for provider config reads.

Observability Requirements: Design lists metrics/logs for rotation progress and failure.

Audit Requirements: Design lists audit events for rotation start, row failure, and completion.

Migration / Rollback Requirements: Design includes rollback and recovery procedure.

Documentation Requirements: Update security operations notes.

Risks: Re-encryption without rollback can permanently lose provider credentials.

Definition of Done: Global DoD applies.

### TH-P5-LOAD-01

Task ID: TH-P5-LOAD-01
Title: Load Failure Harness Scenarios
Phase: P5
Epic: Production Hardening
Type: Test Harness / Reliability
Priority: P2
Dependencies: TH-P05-09, TH-P5-MET-01
Estimate: 1.5d

Objective: Add load and failure scenarios for gateway, wallet, payment worker, and reconciliation review.

Current State: Staging smoke harness is in P0.5, but broader load and failure scenarios are missing.

Scope: Define repeatable scenarios for concurrent gateway requests, Redis outage, DB latency, provider timeout, worker restart, and duplicate payment/reconciliation events.

Out of Scope: Production traffic replay and chaos automation in production.

Implementation Notes: Scenarios must run against staging or local test environments only.

Acceptance Criteria:

- AC-01: Given concurrent gateway load runs, report includes request count, success count, error count, p95 latency, and billing invariant result.
- AC-02: Given Redis outage scenario runs, report shows fail-closed or fallback behavior for affected features.
- AC-03: Given worker restart scenario runs, report shows no duplicate wallet ledger entries.
- AC-04: Given provider timeout scenario runs, report shows retry/compensation path and no sensitive log leakage.

Test Requirements: Load tests; integration failure injection; regression money invariant checks; manual report review.

Observability Requirements: Harness must capture metrics endpoint snapshots before and after each scenario.

Audit Requirements: Harness verifies audit events for denied and manual-review paths.

Migration / Rollback Requirements: No migration. Rollback removes harness scenarios only.

Documentation Requirements: Document scenario execution and expected reports.

Risks: Load tests without isolated environment can affect real balances or providers.

Definition of Done: Global DoD applies.

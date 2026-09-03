# TokenHub Production Alert Runbook (TH-P05-05)

本文档是 TokenHub P0.5 生产告警包（`ops/prometheus/tokenhub_p05.rules.yml`）的配套运行手册。
每条规则注释中的 `runbook` 链接都指向本文档对应小节。所有处置动作必须首先满足下文
“Money-Safety Red Lines（资金安全红线）”。

Scope: this runbook covers exactly the alerts in the P0.5 pack — nothing more.
All rules evaluate only existing Prometheus metrics exported by the API and
worker processes on their `/metrics` endpoints; no log-grep probes exist.

## Release-Blocking Policy

Any **critical** alert in this pack firing in production blocks official-payment
rollout until it is resolved or explicitly waived by a reviewed decision
(see `docs/DEPLOYMENT.md` §10).

## Money-Safety Red Lines

These red lines override every mitigation step in this document. When in doubt,
stop and escalate instead of acting.

This runbook NEVER authorizes:

- direct `UPDATE wallets` statements or direct balance edits in any table
- manual SQL adjustments of balances, ledgers, invoices or billing rows
- batch Spend / debit / credit scripts of any kind
- any automatic correction driven by reconciliation findings
  (reconciliation is **detect-only** by design and must stay that way)

The only allowed money-adjustment path is:

**Diff → Human Review → Explicit Adjustment → Wallet Service → Ledger**

Every adjustment must be an explicit, human-reviewed operation performed through
the Wallet Service so it lands in the ledger like any other money movement.

The P5 Review Workflow is **not implemented yet**. Until it exists: record the
diff (`reconciliation_diffs` id, diff_type, severity, created_at), freeze manual
handling (note who decided what and when), and either wait for the formal
capability or route the adjustment through the existing explicit Wallet Service
adjustment path with the human approval recorded in the diff's resolution notes.
Never improvise a shortcut around this.

## Alert Index

| Alert | Severity | Metric(s) | One-line meaning |
|---|---|---|---|
| TokenHubBillingUndercharge | critical | billing_undercharge_fallback_total | a request settled below its evidence-backed cost |
| TokenHubCriticalReconciliationDiff | critical | reconciliation_critical_diffs_total | reconciliation persisted a critical diff (detect-only) |
| TokenHubWorkerSilentFast | critical | worker_cycles_total | a 60s worker had no successful cycle in 5m |
| TokenHubWorkerSilentReconciler | critical | worker_cycles_total | the reconciler had no successful run in 2h |
| TokenHubWorkerSilentHourly | critical | worker_cycles_total | a subscription worker had no successful run in 90m |
| TokenHubDatabaseUnavailable | critical | app_dependency_up | database unreachable from at least one process |
| TokenHubRedisUnavailable | critical | app_dependency_up | Redis unreachable from at least one process |
| TokenHubWorkerLeaseErrors | warning | worker_lease_total | lease acquisition errors (fail-closed skips) |
| TokenHubPricingIncomplete | warning | billing_pricing_incomplete_total | requests rejected fail-closed for incomplete pricing |

Shared operating context:

- **HA**: leased workers coordinate through Redis leases. One instance holding
  the lease and succeeding while its peer skips (`outcome="skipped"`) is
  HEALTHY leader election. All silence rules aggregate the whole fleet
  (`sum by (worker)`) and fire only when **no** instance succeeds.
- **Labels**: this pack uses only the allowlisted low-cardinality labels
  (endpoint / reason_class / worker / outcome / dependency). Metrics never
  carry identity or secret material; never paste sensitive values into
  tickets or dashboards while working these alerts.
- **Firing tests**: rule semantics are pinned by
  `ops/prometheus/tests/p05_alerts_test.yml` (promtool) and by the structural
  suite in `ops/prometheus/rules_test.go`. If you change a rule, both must stay
  green.

## TokenHubWorkerSilent

The worker-silence family is tiered by schedule because one window cannot fit
all workers:

| Tier | Workers | Schedule | Window | for | Why |
|---|---|---|---|---|---|
| Fast | health_checker, billing_sync | 60s tick / 50s lease TTL | 5m | 2m | 5 schedule ticks of headroom; a late tick or a restart can never fire it |
| Reconciler | reconciler | 1h tick, **no startup cycle** | 2h | 10m | worst-case healthy gap after an unlucky restart is ~2 ticks |
| Hourly | subscription_expirer, subscription_renewer | 1h tick + startup cycle | 90m | 5m | startup cycle makes restarts gap-free; ~30m headroom over worst healthy gap |

All three rules pair an `increase(...) == 0` arm (alive-but-stalled/failing;
reset-safe, so counter resets and process restarts cannot fire it) with an
`absent(...)` arm (process dead / metric never exported; fires after the 5m
staleness window).

### TokenHubWorkerSilentFast

**Severity:** critical · **Expression window:** 5m · **for:** 2m

**Fires when** `sum by (worker)(increase(worker_cycles_total{worker=~"health_checker|billing_sync",outcome="success"}[5m])) == 0`
for 2m, or the worker metric is absent entirely.
**Recovers when** a successful cycle lands anywhere in the fleet (the 5m window
sees an increase again).

**Meaning** — A fast leased worker (health_checker or billing_sync) completed no
successful cycle in the last 5 minutes (>= 5 schedule ticks), or its metrics
are gone entirely.

**Impact** — health_checker silence lets stale dependency state gate traffic
incorrectly; billing_sync silence stalls asynchronous billing settlement work.
Neither loses money directly, but both degrade the money-path safety net.

**First checks**
1. Is the worker process alive and scraped? Fetch its `/metrics` endpoint
   (WORKER_METRICS_ADDR, default 127.0.0.1:19090).
2. Look at `app_dependency_up{dependency="redis"}` and whether
   TokenHubRedisUnavailable / TokenHubWorkerLeaseErrors are also firing —
   when lease acquisition fails, every cycle is fail-closed skipped.
3. Look at `worker_cycles_total{outcome=~"failed|panic_recovered"}` — if it is
   climbing, the worker runs but fails; that is a worker bug, not silence.

**Evidence to inspect**
- `worker_cycles_total{worker=~"health_checker|billing_sync"}` by outcome
- `worker_lease_total{worker=~"health_checker|billing_sync"}` by outcome
- worker process uptime / restart timestamps

**Safe mitigation**
- Restart the worker process if it is wedged. Restart is safe here: leases
  expire (50s TTL), `increase()` tolerates the counter reset, and no cycle is
  double-executed by a restart.
- If a Redis outage is the root cause, restore Redis first
  (see TokenHubRedisUnavailable).
- Never “catch up” by running billing writes from a shell; billing_sync resumes
  its queued work on its own.

**Escalation** — platform/worker on-call immediately; include the billing owner
if billing_sync stays silent beyond 15m.

**Recovery verification** — `sum by (worker)(increase(worker_cycles_total{worker="<name>",outcome="success"}[5m])) > 0`
again and the alert resolves by itself; confirm no lease-error increments
remain.

### TokenHubWorkerSilentReconciler

**Severity:** critical · **Expression window:** 2h · **for:** 10m

**Fires when** the reconciler had no successful cycle in 2h (2 schedule
intervals) or its metric is absent.
**Recovers when** a successful reconciler cycle lands.

**Meaning** — The reconciler (hourly schedule, **no startup cycle** by design)
has not succeeded within two full schedule intervals, or its metrics are gone.
Note: a process restarted at the wrong moment can legitimately take up to ~2h
to produce its next success — check deploy history before treating this as an
incident.

**Impact** — Critical money-safety degradation: reconciliation diffs stop being
detected while this persists. This alert is also the guard for
TokenHubCriticalReconciliationDiff itself — with the reconciler silent, the
critical-diff counter cannot climb.

**First checks**
1. Was the worker process restarted recently (deploy log)? An unlucky restart
   can explain up to ~2h of healthy silence.
2. Is the worker process alive and scraped at all (absent() arm)?
3. Check `worker_lease_total{worker="reconciler"}` — lease errors mean
   fail-closed skips (usually Redis).

**Evidence to inspect**
- `worker_cycles_total{worker="reconciler"}` by outcome
- `worker_lease_total{worker="reconciler"}` by outcome
- last successful reconciliation run timestamp from worker structured logs

**Safe mitigation**
- Restart the worker process to re-arm the schedule. Do NOT hand-run
  reconciliation SQL or “force” a cycle through ad-hoc scripts.
- If Redis lease errors are present, restore Redis first.

**Escalation** — platform on-call; page the money owner as well if silence
exceeds 2h (two missed detection windows).

**Recovery verification** — a successful reconciler cycle appears in
`worker_cycles_total{worker="reconciler",outcome="success"}` and the alert
resolves; verify the next reconciliation run completes without critical diffs
or that any new diffs enter the Human Review path.

### TokenHubWorkerSilentHourly

**Severity:** critical · **Expression window:** 90m · **for:** 5m

**Fires when** subscription_expirer or subscription_renewer had no successful
cycle in 90m, or its metric is absent.
**Recovers when** a successful cycle lands for the affected worker.

**Meaning** — An hourly subscription worker missed more than 1.5 schedule
intervals. Both workers run a startup cycle, so a plain restart can never cause
this alert — silence here is a real stall, lease failure, or dead process.

**Impact** — subscription expirations/renewals lag; user-visible entitlement
drift grows with the silence duration. No direct money loss (renewal billing
goes through the normal fail-closed reserve/settle path).

**First checks**
1. Is the worker process alive and scraped?
2. `worker_lease_total{worker=~"subscription_expirer|subscription_renewer"}` —
   lease errors point at Redis.
3. `worker_cycles_total{outcome=~"failed|panic_recovered"}` for the affected
   worker — running-but-failing is a worker bug.

**Evidence to inspect**
- `worker_cycles_total` for the affected worker, by outcome
- `worker_lease_total` for the affected worker, by outcome
- worker process uptime

**Safe mitigation**
- Restart the worker process; the startup cycle runs immediately, so a restart
  is almost always an instant, safe fix.
- Never trigger subscription state changes with manual SQL.

**Escalation** — platform on-call; include product/billing if silence exceeds
3h.

**Recovery verification** — a successful cycle for each affected worker within
the 90m window; alert resolves by itself.

### TokenHubBillingUndercharge

**Severity:** critical · **Expression window:** 10m · **for:** 1m

**Fires when** `sum(increase(billing_undercharge_fallback_total[10m])) > 0` —
any increase at all. Healthy production never produces these, so the threshold
is deliberately 1, not 100.
**Recovers when** the counter stays flat (no new undercharge within the 10m
window).

**Meaning** — At least one request settled through the settle fallback below
its evidence-backed cost. Money is being under-collected right now.

**Impact** — Direct under-collection. Reconciliation stays detect-only: nothing
re-charges automatically, so exposure persists until humans act.

**First checks**
1. Confirm the increase is real: `sum(increase(billing_undercharge_fallback_total[10m]))`.
2. Check `billing_settle_failed_total` by reason_class and `billing_settle_total`
   for a simultaneous spike (a settle storm points at a systemic fault).
3. Check API structured logs around the settle fallback path for the failing
   provider/model combination (no sensitive data lives in the metrics).

**Evidence to inspect**
- `billing_undercharge_fallback_total` broken down by `endpoint`
- open `reconciliation_diffs` rows with severity=critical,
  diff_type=undercharge_review, resolution_status=open (read-only query)
- pricing catalog completeness for the affected endpoint/model

**Safe mitigation**
- Do NOT re-charge manually. Each occurrence is (or will be, on the next hourly
  run) recorded as a reconciliation diff; leave those rows open for the
  Human Review path.
- If broken pricing data is the driver, fix the pricing catalog through the
  normal configuration path to stop NEW undercharges immediately.
- Follow the red-line path for any recovery:
  Diff → Human Review → Explicit Adjustment → Wallet Service → Ledger.

**Escalation** — page the money/billing owner immediately (critical money
alert). If undercharges continue for more than 30 minutes, escalate to the
engineering lead and consider pausing official-payment traffic at the gateway.

**Recovery verification** —
`sum(increase(billing_undercharge_fallback_total[10m])) == 0` again; every open
undercharge_review diff from the incident window is listed for Human Review;
the alert resolves in Prometheus.

### TokenHubCriticalReconciliationDiff

**Severity:** critical · **Expression window:** 2h · **for:** 1m

**Fires when** `increase(reconciliation_critical_diffs_total[2h]) > 0` — the
reconciler persisted at least one severity=critical diff (undercharge_review or
error_mislabel). Warning-shaped diffs (e.g. idempotent replays missing charge
lines) never increment this counter and can never fire this alert.
**Recovers when** no new critical diff is persisted for 2h. The counter climbs
again on every hourly run while the underlying condition persists, so the alert
stays up until the condition is actually fixed.

**Meaning** — Reconciliation (detect-only) found and persisted a real
money-path inconsistency with critical severity.

**Impact** — A genuine inconsistency exists between what was charged and the
evidence. Nothing auto-corrects; exposure persists until Human Review completes.

**First checks**
1. Read-only query of `reconciliation_diffs` for the newest open critical rows
   (id, diff_type, severity, created_at). Read-only — never UPDATE these rows
   directly.
2. Is TokenHubBillingUndercharge also firing? Then live undercharging is the
   likely source.
3. Confirm the reconciler itself is healthy
   (`worker_cycles_total{worker="reconciler",outcome="success"}`) so detection
   keeps working while you investigate.

**Evidence to inspect**
- the diff rows themselves (diff_type, severity, created_at, resolution_status)
- whether the counter is still climbing hour over hour (condition persists)
- correlated alerts: TokenHubBillingUndercharge, TokenHubWorkerSilentReconciler

**Safe mitigation**
- Follow the red-line path exactly:
  Diff → Human Review → Explicit Adjustment → Wallet Service → Ledger.
- Never run auto-debit/credit logic and never “resolve” a diff with raw SQL.
- Until the P5 Review Workflow exists: record the diff ids, freeze manual
  handling with an explicit owner, and use the Wallet Service explicit
  adjustment path with the approval recorded in the resolution notes.

**Escalation** — money owner immediately. If the diff is an error_mislabel on
settled provider calls, include the provider-integration owner.

**Recovery verification** — the counter stops climbing across the next
reconciler runs; each diff moved through the formal review/adjustment path; the
alert resolves after the 2h window clears.

### TokenHubDatabaseUnavailable

**Severity:** critical · **Expression window:** 1m · **for:** 1m

**Fires when** `min(app_dependency_up{dependency="database"}) == 0` — the 15s
watchdog probe (pool Ping) fails from at least one process for 1m. `min()`
means a single sick process fires the alert.
**Recovers when** every process reports the gauge back to 1.

**Meaning** — At least one running process (API or worker) cannot reach the
primary database.

**Impact** — Money operations fail closed by design: wallet reserves/settles
are rejected, so user requests are rejected rather than undercharged;
reconciliation cannot run; worker DB steps fail. Rejection-during-outage is
the correct money-safe behavior.

**First checks**
1. Database server/provider status: process up, connection limits reached,
   failover or maintenance event.
2. Which process is sick? Scrape each process's `/metrics` individually and
   read `app_dependency_up{dependency="database"}` per instance.
3. Probe `/readyz` on the affected process — expect 503 `not_ready`.

**Evidence to inspect**
- `app_dependency_up{dependency="database"}` per scraped process
- database-side metrics/console (connections, CPU, failover events)
- `/readyz` responses from API and worker endpoints

**Safe mitigation**
- Restore database connectivity (fail back, restart, network/security group
  fix). Do NOT try to bypass the fail-closed gate “to keep traffic flowing” —
  serving requests without a working wallet is exactly how undercharging
  happens.
- No billing catch-up is required: requests rejected before reserve were never
  charged and leave no state to release.

**Escalation** — infrastructure on-call immediately (critical dependency).

**Recovery verification** — `min(app_dependency_up{dependency="database"}) == 1`
for all processes, `/readyz` returns 200, the alert resolves. Afterwards,
confirm the next reconciler run completes and reports no diffs caused by the
outage window.

### TokenHubRedisUnavailable

**Severity:** critical · **Expression window:** 1m · **for:** 1m

**Fires when** `min(app_dependency_up{dependency="redis"}) == 0` for 1m. When
Redis is not configured at all (single-instance dev mode) the series does not
exist and this alert never fires.
**Recovers when** every configured process reports the gauge back to 1.

**Meaning** — At least one process cannot reach Redis (15s watchdog Ping).

**Impact** — Worker leases fail closed: every leased worker skips its cycles
(visible as TokenHubWorkerLeaseErrors, then silence alerts). Rate limiting
degrades. The API itself keeps serving.

**First checks**
1. Redis server status (process, memory, persistence stall).
2. Which process is sick — scrape per-instance
   `app_dependency_up{dependency="redis"}`.
3. Confirm the worker-side fingerprint:
   `worker_lease_total{outcome="error"}` climbing.

**Evidence to inspect**
- `app_dependency_up{dependency="redis"}` per scraped process
- `worker_lease_total` by outcome
- resulting silence alerts (TokenHubWorkerSilent*)

**Safe mitigation**
- Restore Redis. Workers resume automatically once leases can be acquired:
  60s workers within a minute, subscription workers on the next tick (or
  immediately after a restart via their startup cycle). No manual catch-up is
  needed.
- Never disable leasing to work around a Redis outage — leasing is the HA
  safety mechanism against double-execution.

**Escalation** — infrastructure on-call.

**Recovery verification** — gauge back to 1 on all processes; lease-error
increments stop; any consequent worker-silence alerts resolve as successes
resume.

### TokenHubWorkerLeaseErrors

**Severity:** warning · **Expression window:** 10m · **for:** 1m

**Fires when** `sum(increase(worker_lease_total{outcome="error"}[10m])) > 0`.
Healthy HA contention (`outcome="skipped"`) is deliberately never counted here.
**Recovers when** no new lease errors occur within the 10m window.

**Meaning** — Lease acquisition itself is failing (not merely being skipped by
the non-leader). This is the worker-side fingerprint of a Redis outage or a
wedged Redis.

**Impact** — Workers fail-closed skip cycles while errors persist; sustained
errors lead to the worker-silence criticals. Distinguishing “Redis is down”
from “worker bug” is the reason this rule exists separately from
TokenHubRedisUnavailable.

**First checks**
1. Is TokenHubRedisUnavailable firing too? Then treat this as a Redis outage
   and work that runbook section.
2. If Redis looks up: check Redis latency and worker structured logs for
   client-side errors.

**Evidence to inspect**
- `worker_lease_total` by worker/outcome
- `app_dependency_up{dependency="redis"}`
- worker structured logs around lease acquisition

**Safe mitigation**
- Fix the underlying Redis connectivity/latency problem. Never disable or
  loosen leasing to make this alert go away.

**Escalation** — platform on-call (warning level).

**Recovery verification** —
`sum(increase(worker_lease_total{outcome="error"}[10m])) == 0` and worker
cycles resume succeeding.

### TokenHubPricingIncomplete

**Severity:** warning · **Expression window:** 10m · **for:** 1m

**Fires when** `sum(increase(billing_pricing_incomplete_total[10m])) > 0` —
requests were rejected fail-closed before any reserve because model pricing was
incomplete.
**Recovers when** the counter stays flat.

**Meaning** — The money gate worked: requests hit a model/endpoint without
complete pricing and were rejected before any wallet reserve. There is no
direct money loss, but revenue is bleeding and users see errors.

**Impact** — Lost revenue and user-facing failures for every affected request
while the condition persists; no undercharge risk (nothing was reserved).

**First checks**
1. Which endpoint is affected — the metric carries the bounded `endpoint`
   label.
2. Was a new model onboarded without pricing entries? Check the pricing
   catalog for completeness on that model/endpoint.

**Evidence to inspect**
- `billing_pricing_incomplete_total` by endpoint
- `gateway_requests_total` / `gateway_error_total` for the user-visible impact
- pricing catalog entries for the affected endpoint/model

**Safe mitigation**
- Complete the pricing catalog through the normal data/configuration path.
- Never bypass the pricing gate and never hardcode fallback prices in the
  request path — an unpriced request must stay rejected.

**Escalation** — billing/catalog owner (warning level).

**Recovery verification** — the counter stays flat, requests for the affected
endpoint succeed again, and the alert resolves.

## When an alert is not covered here

Every alert the P0.5 pack can raise has a section above. If Prometheus pages
you with a TokenHub alert that has no section in this document, treat that as a
process failure: escalate to the engineering lead and do not improvise money
operations while working it.

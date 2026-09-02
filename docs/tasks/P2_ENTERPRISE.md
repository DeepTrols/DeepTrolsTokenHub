# P2 Enterprise Sprint Tasks

## Epic Code Audit

Existing:

- `tenants`, `tenant_memberships`, and `tenant_invitations` domain models and migrations already exist.
- Tenant status state machine already includes `pending_review`, `active`, `suspended`, `terminated`, and `rejected`.
- Platform admin tenant CRUD and status transition checks exist in console handlers.
- Gateway middleware can resolve active tenant membership into request context.

Partial:

- Admin tenant creation can create owner membership, but self-service application and review workflow are incomplete.
- Invitation storage exists, but enterprise invitation, accept, revoke, and member management APIs are missing.
- Gateway can identify tenant membership, but charged paths still resolve the personal wallet.
- Personal wallet and ledger repositories are mature; enterprise wallet holder semantics are not explicit.

Missing:

- Enterprise wallet schema, repository, service boundaries, reserve, settle, release, top-up, and ledger tests.
- Gateway wallet holder resolution for enterprise members.
- Member monthly limit schema, counters, fail-closed behavior, and concurrency protection.
- Enterprise usage, statement, wallet, members, and review UI flows.

Risks:

- Reusing personal wallet tables without an explicit holder model can corrupt finance boundaries.
- Charging enterprise members to personal wallets creates wrong ledger ownership.
- Monthly limits implemented only in Redis can overspend during outage.
- UI work before API boundaries are stable can hide authorization and audit gaps.

## Split Rule

Every task is sized at 0.5d-2d and can be independently developed, reviewed, tested, accepted, and rolled back. No Enterprise task above 2d remains.

## Enterprise Application And Review

### TH-P2-01

Task ID: TH-P2-01
Title: Enterprise Application Validation
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P1
Dependencies: TH-P05-09
Estimate: 1d

Objective: Define the validation contract for self-service enterprise application creation.

Current State: Admin tenant APIs validate tenant state transitions, but users cannot submit applications through a dedicated self-service API.

Scope: Validate company name, credit code, contact fields, license metadata, and requester identity before persistence.

Out of Scope: Review approval, membership creation, wallet creation, and UI.

Implementation Notes: Reuse existing tenant domain validation where possible and keep request DTOs separate from admin tenant DTOs.

Acceptance Criteria:

- AC-01: Given an authenticated user submits a complete valid payload, validation returns success and normalized fields.
- AC-02: Given an unauthenticated request submits the same payload, validation returns 401 and no domain object is created.
- AC-03: Given `company_name` or credit code is empty, validation returns 400 with a field-specific error.

Test Requirements: Unit validation tests; integration tests for authenticated and unauthenticated path; regression check for existing admin tenant create validation.

Observability Requirements: Log validation failures without license image URLs or sensitive contact data.

Audit Requirements: No audit event until an application is persisted.

Migration / Rollback Requirements: No migration. Rollback removes route and DTO only.

Documentation Requirements: Document request and error schema in enterprise API docs.

Risks: Validation drift from admin tenant validation can create inconsistent enterprise records.

Definition of Done: Global DoD applies.

### TH-P2-02

Task ID: TH-P2-02
Title: Enterprise Application Transaction
Phase: P2
Epic: Enterprise
Type: Backend / Persistence
Priority: P1
Dependencies: TH-P2-01
Estimate: 1.5d

Objective: Persist a valid enterprise application as a pending tenant in a single transaction.

Current State: Admin tenant creation exists; self-service application persistence does not.

Scope: Create pending tenant records with applicant ownership evidence and rollback-safe transaction boundaries.

Out of Scope: Admin approval, invitation, wallet, and notification delivery.

Implementation Notes: Keep the initial state `pending_review` and do not create active memberships before approval.

Acceptance Criteria:

- AC-01: Given a valid application, the API creates exactly one tenant with status `pending_review`.
- AC-02: Given DB insert of tenant metadata fails, the API returns 500 and no partial tenant record remains.
- AC-03: Given the same authenticated user retries after a pre-commit failure, the retry can create one pending application.

Test Requirements: Unit transaction orchestration tests; integration test for tenant insert and rollback; failure injection after validation and before commit.

Observability Requirements: Emit application creation counter by result.

Audit Requirements: Write `enterprise_application.created` with requester, tenant id, and status.

Migration / Rollback Requirements: No migration. Rollback disables the self-service route.

Documentation Requirements: Document initial status and retry behavior.

Risks: Creating active membership too early would bypass review.

Definition of Done: Global DoD applies.

### TH-P2-03

Task ID: TH-P2-03
Title: Enterprise Duplicate Application Guard
Phase: P2
Epic: Enterprise
Type: Backend / Validation
Priority: P1
Dependencies: TH-P2-02
Estimate: 1d

Objective: Prevent duplicate pending enterprise applications for the same applicant or credit code.

Current State: No self-service duplicate guard exists.

Scope: Reject duplicate pending attempts and return the existing pending application status.

Out of Scope: Tenant merging and approved tenant transfer.

Implementation Notes: Use repository lookups or constraints that match existing tenant uniqueness rules.

Acceptance Criteria:

- AC-01: Given a user already has a `pending_review` application, a second request returns 409 and the existing tenant id.
- AC-02: Given a different user submits the same credit code while the first application is pending, the request returns 409.
- AC-03: Given two concurrent identical application requests, exactly one tenant row is committed.

Test Requirements: Unit duplicate decision tests; integration uniqueness tests; concurrency test for simultaneous requests.

Observability Requirements: Emit duplicate rejection counter.

Audit Requirements: Write `enterprise_application.duplicate_rejected` with requester and existing tenant id.

Migration / Rollback Requirements: Migration only if a new partial unique index is required; rollback drops that index.

Documentation Requirements: Document duplicate response shape.

Risks: Improper uniqueness scope can block legitimate re-application after rejection.

Definition of Done: Global DoD applies.

### TH-P2-04

Task ID: TH-P2-04
Title: Enterprise Self-Service Read API
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P1
Dependencies: TH-P2-02
Estimate: 1d

Objective: Allow applicants and enterprise members to read their enterprise application or membership status.

Current State: Admin APIs can list tenants; user-facing enterprise status API is missing.

Scope: Return pending application status for applicants and active tenant summary for members.

Out of Scope: Review actions, wallet data, usage data, and member management.

Implementation Notes: Fail closed when requester has neither application ownership nor active membership.

Acceptance Criteria:

- AC-01: Given an applicant with a pending application, `GET /enterprise/me` returns status `pending_review`.
- AC-02: Given a user with no application and no membership, the API returns 404.
- AC-03: Given a suspended member calls the API, the response includes tenant status and no management permissions.

Test Requirements: Unit resolution tests; integration tests for pending, active, suspended, and none states; regression for admin tenant list.

Observability Requirements: Emit read counter by resolved state.

Audit Requirements: Audit forbidden access attempts only.

Migration / Rollback Requirements: No migration. Rollback removes route.

Documentation Requirements: Document response states.

Risks: Leaking tenant records to unrelated users is a permission defect.

Definition of Done: Global DoD applies.

### TH-P2-05

Task ID: TH-P2-05
Title: Enterprise Review Decision API
Phase: P2
Epic: Enterprise
Type: Backend / Admin API
Priority: P1
Dependencies: TH-P2-02
Estimate: 1.5d

Objective: Create admin approval and rejection actions for pending enterprise applications.

Current State: Admin tenant status updates exist, but they are generic and not tied to application review.

Scope: Approve or reject pending applications and create owner membership on approval.

Out of Scope: Invitation, enterprise wallet creation, and UI.

Implementation Notes: Use existing tenant transition validation and perform tenant update plus owner membership creation atomically.

Acceptance Criteria:

- AC-01: Given a Super Admin approves a `pending_review` application, tenant status becomes `active` and exactly one owner membership exists.
- AC-02: Given an admin without review permission calls approve, the API returns 403 and tenant status remains unchanged.
- AC-03: Given the same approve request is repeated after success, the API returns the current active state and does not create a second owner membership.
- AC-04: Given a request attempts `rejected` to `active` without an allowed transition, the API returns 409.

Test Requirements: Unit state transition tests; integration transaction tests; concurrency test for two approval requests.

Observability Requirements: Emit review action counter by decision and result.

Audit Requirements: Write `enterprise_application.reviewed` with operator, decision, reason, before status, and after status.

Migration / Rollback Requirements: No migration. Rollback disables review-specific route.

Documentation Requirements: Document allowed statuses and idempotent approval response.

Risks: Non-atomic owner membership creation can leave an active tenant without an owner.

Definition of Done: Global DoD applies.

### TH-P2-06

Task ID: TH-P2-06
Title: Enterprise Review Audit Snapshot
Phase: P2
Epic: Enterprise
Type: Audit / Backend
Priority: P1
Dependencies: TH-P2-05
Estimate: 1d

Objective: Persist review evidence snapshots so later tenant edits do not change review history.

Current State: Generic audit middleware exists, but enterprise review evidence is not explicitly snapshotted.

Scope: Capture review decision, operator, submitted fields, reason, and timestamps in an immutable audit payload.

Out of Scope: Document storage, UI diff viewer, and notification.

Implementation Notes: Do not store secrets or signed private asset URLs in audit payloads.

Acceptance Criteria:

- AC-01: Given an approval succeeds, the audit entry contains submitted company fields and approval reason.
- AC-02: Given tenant fields are edited later, the existing review audit payload remains unchanged.
- AC-03: Given a review action fails validation, no success audit event is written.

Test Requirements: Unit audit payload tests; integration audit persistence; failure injection for audit write failure.

Observability Requirements: Emit audit write failure counter.

Audit Requirements: This task defines the review audit snapshot.

Migration / Rollback Requirements: No migration unless audit event type enum exists; rollback removes event type use.

Documentation Requirements: Document audit fields and privacy exclusions.

Risks: Storing full license artifacts in audit can leak sensitive business documents.

Definition of Done: Global DoD applies.

## Invitation And Membership

### TH-P2-INV-01

Task ID: TH-P2-INV-01
Title: Invitation Schema Gap Review
Phase: P2
Epic: Enterprise
Type: Backend / Design
Priority: P1
Dependencies: TH-P2-02
Estimate: 0.5d

Objective: Confirm whether existing `tenant_invitations` columns cover enterprise invitation lifecycle.

Current State: `tenant_invitations` exists, but no active API uses it for enterprise onboarding.

Scope: Review token hash, expiry, inviter, role, status, accepted timestamp, and uniqueness requirements.

Out of Scope: API implementation and email delivery.

Implementation Notes: Prefer additive migrations if lifecycle fields are missing.

Acceptance Criteria:

- AC-01: Given the review is complete, the task outputs a column-by-column decision of reuse, add, or ignore.
- AC-02: Given a required lifecycle field is missing, a migration task or note is linked from this task.
- AC-03: Given existing referral invite code tables are found, the review explicitly marks them separate from tenant invitations.

Test Requirements: Integration schema inspection; regression check that referral invitation tests are untouched; manual reviewer signoff.

Observability Requirements: No runtime metric required.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: This task may create only a migration plan; actual migration is separate if needed.

Documentation Requirements: Record schema decision in enterprise implementation notes.

Risks: Confusing referral invites with tenant invitations can expose membership joins incorrectly.

Definition of Done: Global DoD applies.

### TH-P2-INV-02

Task ID: TH-P2-INV-02
Title: Create Invitation API
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P1
Dependencies: TH-P2-INV-01, TH-P2-05
Estimate: 1d

Objective: Allow enterprise owner/admin users to create member invitation tokens.

Current State: Invitation storage exists, but no enterprise create API exists.

Scope: Create invitation records with role, expiry, inviter, tenant id, and single-use token hash.

Out of Scope: Email sending, accept flow, and UI.

Implementation Notes: Store only token hashes. Return the raw token once in the creation response.

Acceptance Criteria:

- AC-01: Given an active owner creates an invitation for role `member`, the API returns a one-time raw token and stores only a hash.
- AC-02: Given an unauthenticated request calls the route, it returns 401 and no invitation is created.
- AC-03: Given an active member without invitation permission calls the route, it returns 403 and no invitation is created.
- AC-04: Given the tenant status is not `active`, the API returns 409.

Test Requirements: Unit role validation; integration hash persistence and permission matrix; failure injection for DB insert failure.

Observability Requirements: Emit invitation creation counter by role and result.

Audit Requirements: Write `enterprise_invitation.created` with inviter, tenant, role, and expiry.

Migration / Rollback Requirements: Rollback disables the route; migration only if TH-P2-INV-01 requires columns.

Documentation Requirements: Document token one-time visibility.

Risks: Returning stored raw tokens would create credential exposure.

Definition of Done: Global DoD applies.

### TH-P2-INV-03

Task ID: TH-P2-INV-03
Title: List And Revoke Invitation API
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P1
Dependencies: TH-P2-INV-02
Estimate: 1d

Objective: Allow owner/admin users to view pending invitations and revoke unused tokens.

Current State: No enterprise invitation listing or revocation API exists.

Scope: List invitations with masked token state and revoke pending unaccepted invitations.

Out of Scope: Accept flow and notification.

Implementation Notes: Do not return raw token values after creation.

Acceptance Criteria:

- AC-01: Given an owner lists invitations, each row includes role, status, inviter, expiry, and no raw token.
- AC-02: Given a pending invitation is revoked, its status becomes `revoked` and later accept attempts fail.
- AC-03: Given an accepted invitation is revoked, the API returns 409 and membership remains active.

Test Requirements: Unit revocation state tests; integration list/revoke permissions; regression for create response.

Observability Requirements: Emit revoke counter by result.

Audit Requirements: Write `enterprise_invitation.revoked` with operator and invitation id.

Migration / Rollback Requirements: No migration unless status enum expansion is required.

Documentation Requirements: Document revocation state behavior.

Risks: Returning raw tokens in list responses would expose join credentials.

Definition of Done: Global DoD applies.

### TH-P2-INV-04

Task ID: TH-P2-INV-04
Title: Accept Invitation Token API
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P1
Dependencies: TH-P2-INV-02
Estimate: 1.5d

Objective: Allow authenticated users to join an active enterprise using a valid invitation token.

Current State: No enterprise invitation acceptance flow exists.

Scope: Validate token hash, expiry, status, tenant status, duplicate membership, and single-use acceptance transaction.

Out of Scope: Email landing page and member UI.

Implementation Notes: Acceptance must update invitation and create membership atomically.

Acceptance Criteria:

- AC-01: Given an authenticated user accepts a valid unexpired token for an active tenant, one active membership is created and invitation status becomes `accepted`.
- AC-02: Given the same token is submitted twice, the second request returns 409 and no second membership is created.
- AC-03: Given two users submit the same token concurrently, exactly one membership is committed.
- AC-04: Given the tenant is suspended, acceptance returns 409 and invitation remains unused.

Test Requirements: Unit token status matrix; integration transactional accept path; concurrency test for duplicate token acceptance.

Observability Requirements: Emit accept counter by result and failure reason.

Audit Requirements: Write `enterprise_invitation.accepted` with user, tenant, and invitation id.

Migration / Rollback Requirements: No migration unless TH-P2-INV-01 requires lifecycle fields.

Documentation Requirements: Document token failure cases.

Risks: Non-atomic acceptance can consume an invitation without creating membership.

Definition of Done: Global DoD applies.

### TH-P2-INV-05

Task ID: TH-P2-INV-05
Title: Membership Role Update Guard
Phase: P2
Epic: Enterprise
Type: Backend / Authorization
Priority: P1
Dependencies: TH-P2-INV-04
Estimate: 1d

Objective: Allow owner users to update member roles without creating owner lockout.

Current State: Membership repository has role update capability, but enterprise role update API is missing.

Scope: Implement role update authorization, role transition checks, and last-owner protection.

Out of Scope: Invitation creation and UI.

Implementation Notes: Owner demotion must fail when it would leave the tenant with no active owner.

Acceptance Criteria:

- AC-01: Given an owner promotes a member to admin, the membership role changes to `admin`.
- AC-02: Given an admin tries to promote another user to owner, the API returns 403.
- AC-03: Given the only active owner tries to demote themself, the API returns 409 and role remains `owner`.
- AC-04: Given an unauthenticated request calls the route, the API returns 401.

Test Requirements: Unit role matrix; integration permission tests; concurrency test for owner demotion.

Observability Requirements: Emit role update counter by result.

Audit Requirements: Write `enterprise_membership.role_updated` with operator, target, before role, and after role.

Migration / Rollback Requirements: No migration. Rollback disables role update route.

Documentation Requirements: Document role permissions and self-lockout rule.

Risks: Owner lockout would make enterprise administration unrecoverable without direct DB access.

Definition of Done: Global DoD applies.

### TH-P2-INV-06

Task ID: TH-P2-INV-06
Title: Member Removal Owner Guard
Phase: P2
Epic: Enterprise
Type: Backend / Authorization
Priority: P1
Dependencies: TH-P2-INV-04
Estimate: 1d

Objective: Allow owner/admin users to remove members while protecting tenant ownership.

Current State: Membership statuses exist, but enterprise removal API is missing.

Scope: Mark members as left or suspended according to product decision and block removal of the last owner.

Out of Scope: Hard deletion and billing history rewrites.

Implementation Notes: Preserve historical usage ownership even after removal.

Acceptance Criteria:

- AC-01: Given an owner removes an active member, membership status becomes non-active and gateway access for that member returns forbidden.
- AC-02: Given an admin removes an owner, the API returns 403 and membership remains active.
- AC-03: Given the only active owner tries to remove themself, the API returns 409 and membership remains active.

Test Requirements: Unit removal authorization matrix; integration removed-member access checks; concurrency test for owner removal.

Observability Requirements: Emit member removal counter by result.

Audit Requirements: Write `enterprise_membership.removed` with operator, target, and reason.

Migration / Rollback Requirements: No migration. Rollback disables removal route.

Documentation Requirements: Document removal state and history retention.

Risks: Hard deletion can break ledger and usage audit chains.

Definition of Done: Global DoD applies.

## Enterprise Wallet

### TH-P2-WAL-01

Task ID: TH-P2-WAL-01
Title: Enterprise Wallet Holder Design Record
Phase: P2
Epic: Enterprise
Type: Finance / Design
Priority: P1
Dependencies: TH-P2-05
Estimate: 0.5d

Objective: Decide and document the enterprise wallet holder model before writing money migrations.

Current State: Personal wallet tables are mature, but `wallets.user_id` is not a neutral holder key.

Scope: Choose explicit enterprise wallet storage, transaction relation, idempotency key scope, and rollback plan.

Out of Scope: Migration implementation and gateway integration.

Implementation Notes: The decision must preserve personal wallet behavior and ledger auditability.

Acceptance Criteria:

- AC-01: Given the design is reviewed, it states whether enterprise wallet transactions use a new ledger table or an explicit holder discriminator.
- AC-02: Given the selected design touches `wallet_transactions`, it lists required FK and uniqueness changes.
- AC-03: Given rollback is reviewed, it states how to disable enterprise wallet charging without affecting personal wallets.

Test Requirements: Integration schema feasibility review; regression checklist for personal wallet invariants; manual finance and backend signoff.

Observability Requirements: No runtime metric required.

Audit Requirements: Design must list required audit event names.

Migration / Rollback Requirements: Design must include migration and rollback notes.

Documentation Requirements: Record the decision in enterprise finance docs.

Risks: Skipping this decision can blend personal and enterprise balances.

Definition of Done: Global DoD applies.

### TH-P2-WAL-02

Task ID: TH-P2-WAL-02
Title: Enterprise Wallet Migration
Phase: P2
Epic: Enterprise
Type: Finance / Migration
Priority: P1
Dependencies: TH-P2-WAL-01
Estimate: 1d

Objective: Create the enterprise wallet schema selected by the holder design record.

Current State: No production-ready enterprise wallet schema exists.

Scope: Add wallet balance, version, timestamps, tenant ownership, and required ledger linkage.

Out of Scope: Repository methods and gateway charging.

Implementation Notes: Use decimal-safe numeric columns consistent with personal wallets.

Acceptance Criteria:

- AC-01: Given migration runs on a clean DB, enterprise wallet tables and constraints are created.
- AC-02: Given rollback runs immediately after migration, all new enterprise wallet schema objects are removed.
- AC-03: Given migration runs on a DB with personal wallet rows, personal wallet balances and transactions remain unchanged.

Test Requirements: Integration migration up/down on clean and seeded DB; regression for personal wallet repository tests; failure injection for migration failure.

Observability Requirements: No runtime metric required.

Audit Requirements: No audit event required for schema migration.

Migration / Rollback Requirements: Migration and rollback scripts are required and tested.

Documentation Requirements: Update migration notes.

Risks: Wrong FK direction can prevent historical ledger retention.

Definition of Done: Global DoD applies.

### TH-P2-WAL-03

Task ID: TH-P2-WAL-03
Title: Enterprise Wallet Repository FindCreate
Phase: P2
Epic: Enterprise
Type: Finance / Repository
Priority: P1
Dependencies: TH-P2-WAL-02
Estimate: 1d

Objective: Create repository methods for finding or creating one enterprise wallet per tenant.

Current State: Personal wallet repository has find/create behavior; enterprise equivalent is missing.

Scope: Implement `FindByTenant`, `CreateForTenant`, and idempotent `FindOrCreateForTenant`.

Out of Scope: Top-up, reserve, settle, release, and UI.

Implementation Notes: Use transaction boundaries and uniqueness constraints to prevent duplicate wallets.

Acceptance Criteria:

- AC-01: Given a tenant without wallet calls find-or-create, exactly one enterprise wallet is created.
- AC-02: Given two concurrent find-or-create calls for the same tenant, both return the same wallet id.
- AC-03: Given DB insert fails, the method returns an error and no partial ledger row is created.

Test Requirements: Unit repository error mapping; integration unique tenant wallet tests; concurrency test for simultaneous creation.

Observability Requirements: Emit repository-level wallet create result metric if existing pattern permits.

Audit Requirements: No audit event for internal find-or-create.

Migration / Rollback Requirements: No migration beyond TH-P2-WAL-02. Rollback removes repository usage.

Documentation Requirements: Document one-wallet-per-tenant invariant.

Risks: Duplicate enterprise wallets make balance resolution ambiguous.

Definition of Done: Global DoD applies.

### TH-P2-WAL-04

Task ID: TH-P2-WAL-04
Title: Enterprise Wallet TopUp Ledger
Phase: P2
Epic: Enterprise
Type: Finance / Service
Priority: P1
Dependencies: TH-P2-WAL-03
Estimate: 1.5d

Objective: Implement enterprise wallet top-up with ledger entry and idempotency.

Current State: Personal wallet top-up ledger exists; enterprise top-up path is missing.

Scope: Add top-up operation, ledger transaction, balance update, operator/request metadata, and idempotency key handling.

Out of Scope: Official payment provider integration and admin UI.

Implementation Notes: Follow personal wallet decimal and optimistic-locking patterns.

Acceptance Criteria:

- AC-01: Given a valid top-up of 100.00, enterprise wallet balance increases by 100.00 and one top-up ledger entry is created.
- AC-02: Given the same idempotency key is submitted twice, balance changes once and the same transaction is returned.
- AC-03: Given ledger insert fails after balance update is attempted, the transaction rolls back and balance remains unchanged.
- AC-04: Given amount is zero or negative, the operation returns 400 and no ledger entry is created.

Test Requirements: Unit amount/idempotency tests; integration balance plus ledger tests; failure injection for ledger insert failure.

Observability Requirements: Emit top-up counter by result and amount bucket.

Audit Requirements: Write `enterprise_wallet.topup` with operator, tenant, amount, before balance, and after balance.

Migration / Rollback Requirements: No migration beyond wallet schema. Rollback disables enterprise top-up caller.

Documentation Requirements: Document idempotency key requirement.

Risks: Balance update without ledger entry creates账外资金.

Definition of Done: Global DoD applies.

### TH-P2-WAL-05

Task ID: TH-P2-WAL-05
Title: Enterprise Wallet Reserve
Phase: P2
Epic: Enterprise
Type: Finance / Service
Priority: P1
Dependencies: TH-P2-WAL-03
Estimate: 1.5d

Objective: Reserve enterprise wallet funds before upstream gateway execution.

Current State: Personal wallet reserve exists; enterprise reserve path is missing.

Scope: Reserve funds with idempotency, insufficient balance behavior, DB error handling, and optimistic locking.

Out of Scope: Settle, release, gateway route integration, and monthly limits.

Implementation Notes: Reserve amount must be a positive decimal and must not allow negative available balance.

Acceptance Criteria:

- AC-01: Given wallet balance is 10.00 and reserve amount is 6.00, available balance decreases by 6.00 and one reserve ledger entry is created.
- AC-02: Given wallet balance is 5.00 and reserve amount is 6.00, the operation returns insufficient balance and no ledger entry is created.
- AC-03: Given two concurrent reserve requests of 6.00 against balance 10.00, at most one succeeds.
- AC-04: Given the same idempotency key is retried, the original reserve transaction is returned and balance is not decremented again.

Test Requirements: Unit insufficient balance/idempotency tests; integration reserve ledger tests; concurrency test for competing reserves.

Observability Requirements: Emit reserve success, insufficient balance, DB error, and idempotent replay metrics.

Audit Requirements: Reserve ledger entry must include tenant and idempotency key.

Migration / Rollback Requirements: No migration beyond wallet schema. Rollback stops enterprise gateway charging from calling reserve.

Documentation Requirements: Document insufficient balance response.

Risks: Race conditions can allow enterprise wallet overspend.

Definition of Done: Global DoD applies.

### TH-P2-WAL-06

Task ID: TH-P2-WAL-06
Title: Enterprise Wallet Settle
Phase: P2
Epic: Enterprise
Type: Finance / Service
Priority: P1
Dependencies: TH-P2-WAL-05
Estimate: 1.5d

Objective: Commit an enterprise wallet reserve to the final charged amount.

Current State: Enterprise reserve transaction does not yet have a settle path.

Scope: Settle reserved transactions with final amount, ledger consistency, idempotency, and partial failure rollback.

Out of Scope: Gateway endpoint integration and reconciliation review.

Implementation Notes: Final amount must be less than or equal to reserved amount unless a separate review-approved adjustment exists.

Acceptance Criteria:

- AC-01: Given a 10.00 reserve settles at 7.00, reserved amount is cleared, 7.00 is committed, and 3.00 is released.
- AC-02: Given the same settle idempotency key is retried, balance and ledger remain unchanged after the first success.
- AC-03: Given final amount exceeds reserved amount, the operation returns 409 and no additional debit is made.
- AC-04: Given commit ledger write fails, the reserve remains recoverable and no partial final debit is visible.

Test Requirements: Unit final amount/idempotency matrix; integration settle ledger tests; failure injection for ledger and balance failure.

Observability Requirements: Emit settle result metrics and amount difference histogram.

Audit Requirements: Ledger must link reserve transaction, request id, and final charge.

Migration / Rollback Requirements: No migration beyond wallet schema. Rollback disables callers of settle.

Documentation Requirements: Document final amount limit and review adjustment boundary.

Risks: Allowing settle above reserve can recreate B5 undercharge or overcharge ambiguity.

Definition of Done: Global DoD applies.

### TH-P2-WAL-07

Task ID: TH-P2-WAL-07
Title: Enterprise Wallet Release
Phase: P2
Epic: Enterprise
Type: Finance / Service
Priority: P1
Dependencies: TH-P2-WAL-05
Estimate: 1d

Objective: Release enterprise wallet reserves when upstream execution fails or is cancelled.

Current State: Enterprise reserve has no release operation.

Scope: Release reserved transactions idempotently and preserve ledger evidence.

Out of Scope: Settle and gateway route integration.

Implementation Notes: Release must never delete the original reserve ledger evidence.

Acceptance Criteria:

- AC-01: Given a 6.00 reserve is released, available balance increases by 6.00 and one release ledger entry is created.
- AC-02: Given the same release request is repeated, balance changes once and the original release result is returned.
- AC-03: Given a settled reserve is released, the operation returns 409 and no balance change occurs.

Test Requirements: Unit reserve state tests; integration release ledger tests; failure injection for DB error.

Observability Requirements: Emit release result metrics.

Audit Requirements: Ledger must link release transaction to original reserve.

Migration / Rollback Requirements: No migration beyond wallet schema. Rollback disables enterprise release callers.

Documentation Requirements: Document release idempotency behavior.

Risks: Double release can inflate enterprise balances.

Definition of Done: Global DoD applies.

### TH-P2-WAL-08

Task ID: TH-P2-WAL-08
Title: Enterprise Wallet Transaction Listing
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P1
Dependencies: TH-P2-WAL-04, TH-P2-WAL-06
Estimate: 1d

Objective: Expose enterprise wallet balance and ledger history to authorized enterprise managers.

Current State: Personal wallet APIs exist; enterprise wallet read API is missing.

Scope: Return balance, paginated transactions, transaction type, amount, request id, operator, and timestamps.

Out of Scope: Top-up UI and statement aggregation.

Implementation Notes: Members without owner/admin role must not see enterprise wallet ledger.

Acceptance Criteria:

- AC-01: Given an owner requests wallet history, the response returns balance and paginated ledger rows.
- AC-02: Given a regular member requests wallet history, the API returns 403.
- AC-03: Given an unauthenticated request calls the route, the API returns 401.

Test Requirements: Unit pagination/role tests; integration owner/admin/member permission cases; regression for personal wallet API.

Observability Requirements: Emit wallet read counter by role and result.

Audit Requirements: Audit forbidden wallet read attempts.

Migration / Rollback Requirements: No migration. Rollback disables read route.

Documentation Requirements: Document response fields and redaction rules.

Risks: Ledger visibility to regular members can expose company spend.

Definition of Done: Global DoD applies.

### TH-P2-WAL-09

Task ID: TH-P2-WAL-09
Title: Enterprise Wallet Ledger Consistency Tests
Phase: P2
Epic: Enterprise
Type: Test / Finance
Priority: P1
Dependencies: TH-P2-WAL-04, TH-P2-WAL-06, TH-P2-WAL-07
Estimate: 1.5d

Objective: Create a regression suite proving enterprise wallet balances match ledger entries.

Current State: Personal wallet tests exist; enterprise ledger consistency tests are missing.

Scope: Cover top-up, reserve, settle, release, duplicate request, insufficient balance, concurrency, DB exception, and partial failure.

Out of Scope: Gateway endpoint tests and monthly limit tests.

Implementation Notes: Use deterministic decimal assertions and transaction-level failure injection.

Acceptance Criteria:

- AC-01: Given any successful top-up/reserve/settle/release sequence, computed ledger balance equals stored wallet balance.
- AC-02: Given duplicate idempotency keys are replayed, computed ledger balance equals the first successful result.
- AC-03: Given injected DB failure at each money write point, no sequence produces a stored balance that differs from ledger sum.
- AC-04: Given concurrent reserve attempts exceed available balance, ledger shows no negative available balance.

Test Requirements: Unit invariant calculator tests; integration money flow tests; concurrency and failure injection tests.

Observability Requirements: No new runtime metric required; failed invariant tests must print request ids only.

Audit Requirements: Test fixtures must verify audit/ledger metadata presence.

Migration / Rollback Requirements: No migration. Rollback removes test files only.

Documentation Requirements: Document invariant formulas.

Risks: Without this suite, enterprise wallet regressions can pass normal API tests.

Definition of Done: Global DoD applies.

## Gateway Enterprise Charging

### TH-P2-GW-01

Task ID: TH-P2-GW-01
Title: Wallet Holder Resolver
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-WAL-03, TH-P2-INV-04
Estimate: 1d

Objective: Resolve whether a gateway request should charge a personal wallet or an enterprise wallet.

Current State: Gateway membership context exists, but charged paths call personal wallet lookup.

Scope: Create a resolver that returns holder type, holder id, tenant id when present, and wallet id.

Out of Scope: Reserve/settle integration and monthly limit checks.

Implementation Notes: Fail closed if enterprise context is ambiguous or tenant status is not active.

Acceptance Criteria:

- AC-01: Given a user without active enterprise membership, resolver returns personal wallet holder.
- AC-02: Given an active enterprise member with active tenant, resolver returns enterprise wallet holder.
- AC-03: Given enterprise membership exists but tenant is suspended, resolver returns fail-closed error and no wallet id.

Test Requirements: Unit holder matrix; integration repository lookups; regression for personal gateway charging.

Observability Requirements: Emit holder resolution counter by holder type and result.

Audit Requirements: Audit fail-closed enterprise resolution events.

Migration / Rollback Requirements: No migration. Rollback keeps gateway on personal wallet path.

Documentation Requirements: Document holder precedence rules.

Risks: Ambiguous holder resolution can charge the wrong account.

Definition of Done: Global DoD applies.

### TH-P2-GW-02

Task ID: TH-P2-GW-02
Title: Enterprise Status Fail-Closed
Phase: P2
Epic: Enterprise
Type: Gateway / Safety
Priority: P1
Dependencies: TH-P2-GW-01
Estimate: 1d

Objective: Reject enterprise gateway calls when tenant or membership state is not allowed.

Current State: Tenant status state machine exists, but gateway charged paths do not enforce enterprise status consistently.

Scope: Map tenant and membership states to explicit gateway allow/deny outcomes.

Out of Scope: Wallet reserve and UI.

Implementation Notes: Use deny-by-default behavior for unknown tenant or membership status.

Acceptance Criteria:

- AC-01: Given tenant status is `active` and membership status is `active`, gateway charging can proceed to wallet resolution.
- AC-02: Given tenant status is `pending_review`, `suspended`, `terminated`, or `rejected`, gateway returns 403 before upstream call.
- AC-03: Given membership status is not active, gateway returns 403 before wallet reserve.
- AC-04: Given tenant lookup returns DB error, gateway returns 503 and does not call upstream.

Test Requirements: Unit status matrix; integration gateway denial tests; failure injection for tenant lookup DB error.

Observability Requirements: Emit fail-closed counter by blocked state.

Audit Requirements: Write audit event for enterprise gateway denial with reason and no prompt body.

Migration / Rollback Requirements: No migration. Rollback removes enterprise status check from gateway.

Documentation Requirements: Document denied states.

Risks: Fail-open behavior can spend enterprise funds after suspension.

Definition of Done: Global DoD applies.

### TH-P2-GW-03

Task ID: TH-P2-GW-03
Title: Chat Endpoint Enterprise Reserve Integration
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-GW-02, TH-P2-WAL-05
Estimate: 1.5d

Objective: Route chat completion reserve calls to enterprise wallet for active enterprise members.

Current State: Chat gateway charges personal wallet after resolving the user.

Scope: Integrate holder resolver with chat endpoint reserve path and preserve personal behavior.

Out of Scope: Other endpoints, monthly limits, and UI.

Implementation Notes: Reserve must occur before upstream execution.

Acceptance Criteria:

- AC-01: Given an active enterprise member calls chat, enterprise reserve is called before upstream execution.
- AC-02: Given enterprise wallet has insufficient balance, gateway returns 402 and upstream is not called.
- AC-03: Given enterprise wallet reserve returns DB error, gateway returns 503 and upstream is not called.
- AC-04: Given a personal user calls chat, personal reserve path is used.

Test Requirements: Unit holder branching tests; integration chat enterprise reserve tests; failure injection for reserve DB error.

Observability Requirements: Emit chat reserve metrics with holder type.

Audit Requirements: Ledger must include request id and holder type.

Migration / Rollback Requirements: No migration. Rollback switches chat endpoint to personal-only resolver.

Documentation Requirements: Document enterprise charging behavior for chat.

Risks: Calling upstream before reserve can create uncharged provider cost.

Definition of Done: Global DoD applies.

### TH-P2-GW-04

Task ID: TH-P2-GW-04
Title: Responses And Claude Enterprise Reserve Integration
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-GW-03
Estimate: 1.5d

Objective: Apply enterprise wallet reserve behavior to Responses and Claude relay endpoints.

Current State: These endpoints have separate gateway paths that must not be assumed covered by chat changes.

Scope: Wire holder resolver and reserve call for Responses and Claude relay.

Out of Scope: Embeddings, images, audio, video, and UI.

Implementation Notes: Keep endpoint-specific request id and idempotency key patterns.

Acceptance Criteria:

- AC-01: Given an enterprise member calls Responses endpoint, enterprise reserve happens before upstream call.
- AC-02: Given an enterprise member calls Claude relay, enterprise reserve happens before upstream call.
- AC-03: Given reserve fails on either endpoint, upstream is not called and no charged usage log is committed.

Test Requirements: Unit resolver integration tests; integration Responses and Claude reserve path tests; failure injection for reserve failure.

Observability Requirements: Emit endpoint and holder type labels on reserve metrics.

Audit Requirements: Ledger entries must carry endpoint identifier.

Migration / Rollback Requirements: No migration. Rollback removes enterprise resolver from these endpoints.

Documentation Requirements: Update enterprise gateway endpoint matrix.

Risks: Endpoint gaps can silently charge personal wallets for enterprise usage.

Definition of Done: Global DoD applies.

### TH-P2-GW-05

Task ID: TH-P2-GW-05
Title: Embeddings And Images Enterprise Reserve Integration
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-GW-03
Estimate: 1.5d

Objective: Apply enterprise wallet reserve behavior to embeddings and image endpoints.

Current State: Non-chat endpoints have separate handlers and cost units.

Scope: Wire holder resolver, reserve amount calculation, and failure behavior for embeddings and images.

Out of Scope: Audio, video, and monthly limits.

Implementation Notes: Use endpoint-specific pricing units and keep idempotency key format deterministic.

Acceptance Criteria:

- AC-01: Given an enterprise member calls embeddings, reserve uses enterprise wallet and correct pricing unit.
- AC-02: Given an enterprise member calls image generation, reserve uses enterprise wallet and correct pricing unit.
- AC-03: Given reserve fails, upstream is not called and no charged usage is written.

Test Requirements: Unit reserve amount tests; integration embeddings and images endpoint tests; failure injection for reserve failure.

Observability Requirements: Emit endpoint, holder type, and pricing unit labels.

Audit Requirements: Ledger entries must include request id and endpoint type.

Migration / Rollback Requirements: No migration. Rollback removes enterprise resolver from these endpoints.

Documentation Requirements: Update endpoint coverage matrix.

Risks: Pricing unit mismatch can create undercharge or overcharge.

Definition of Done: Global DoD applies.

### TH-P2-GW-06

Task ID: TH-P2-GW-06
Title: Audio And Video Enterprise Reserve Integration
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-GW-03
Estimate: 1.5d

Objective: Apply enterprise wallet reserve behavior to audio and video endpoints.

Current State: Audio and video flows have distinct pricing and execution paths.

Scope: Wire holder resolver, reserve amount calculation, and pre-upstream failure behavior for audio and video.

Out of Scope: Embeddings, images, UI, and monthly limits.

Implementation Notes: Preserve endpoint-specific cost estimation and request id handling.

Acceptance Criteria:

- AC-01: Given an enterprise member calls audio endpoint, enterprise reserve happens before upstream.
- AC-02: Given an enterprise member calls video endpoint, enterprise reserve happens before upstream.
- AC-03: Given DB error during reserve, gateway returns 503 and upstream is not called.

Test Requirements: Unit holder and pricing unit tests; integration audio and video endpoint tests; failure injection for reserve DB error.

Observability Requirements: Emit endpoint and holder type labels.

Audit Requirements: Ledger entries must link to request id and endpoint.

Migration / Rollback Requirements: No migration. Rollback removes enterprise resolver from these endpoints.

Documentation Requirements: Update endpoint coverage matrix.

Risks: Endpoint omissions create inconsistent enterprise billing.

Definition of Done: Global DoD applies.

### TH-P2-GW-07

Task ID: TH-P2-GW-07
Title: Enterprise Settle Release Endpoint Matrix
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-GW-03, TH-P2-GW-04, TH-P2-GW-05, TH-P2-GW-06, TH-P2-WAL-06, TH-P2-WAL-07
Estimate: 1.5d

Objective: Ensure every enterprise gateway endpoint settles successful requests and releases failed requests.

Current State: Reserve integration is split by endpoint; final charge/release matrix is not yet proven.

Scope: Cover chat, Responses, Claude, embeddings, images, audio, and video success/failure paths.

Out of Scope: Monthly member limits and UI.

Implementation Notes: Use existing B5 safety rules: final settle must not exceed reserved amount.

Acceptance Criteria:

- AC-01: Given a successful enterprise request with parsed usage, the endpoint calls enterprise settle exactly once.
- AC-02: Given upstream fails before billable usage, the endpoint calls enterprise release exactly once.
- AC-03: Given usage parsing fails after upstream success, the endpoint follows B5 fallback behavior and emits reconciliation evidence.
- AC-04: Given a duplicate retry with same idempotency key, no duplicate enterprise debit is created.

Test Requirements: Unit endpoint outcome matrix; integration across all endpoint categories; failure injection for usage parse failure.

Observability Requirements: Emit settle/release metrics by endpoint and holder type.

Audit Requirements: Ledger and usage records must preserve request id linkage.

Migration / Rollback Requirements: No migration. Rollback disables enterprise holder integration per endpoint.

Documentation Requirements: Document endpoint charging matrix.

Risks: Missing a release path can strand reserved enterprise balance.

Definition of Done: Global DoD applies.

## Member Monthly Limit

### TH-P2-LIM-01

Task ID: TH-P2-LIM-01
Title: Monthly Limit Schema
Phase: P2
Epic: Enterprise
Type: Finance / Migration
Priority: P1
Dependencies: TH-P2-INV-01
Estimate: 0.5d

Objective: Add schema for per-member monthly spend limits.

Current State: Tenant membership exists, but no monthly limit or period spend schema exists.

Scope: Create member limit fields or tables, period identifier rules, and uniqueness constraints.

Out of Scope: Redis counters, gateway enforcement, and UI.

Implementation Notes: Use currency-safe decimal columns and explicit timezone rules.

Acceptance Criteria:

- AC-01: Given migration runs on clean DB, monthly limit schema is created with tenant and member references.
- AC-02: Given rollback runs, new monthly limit schema objects are removed.
- AC-03: Given duplicate limit rows for the same member and month are inserted, DB constraint rejects the duplicate.

Test Requirements: Integration migration up/down and uniqueness tests; regression for tenant membership migration tests.

Observability Requirements: No runtime metric required.

Audit Requirements: No audit event for schema migration.

Migration / Rollback Requirements: Migration and rollback are required and tested.

Documentation Requirements: Document monthly period timezone.

Risks: Ambiguous period boundaries can misstate monthly spend.

Definition of Done: Global DoD applies.

### TH-P2-LIM-02

Task ID: TH-P2-LIM-02
Title: Monthly Limit Admin API
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P1
Dependencies: TH-P2-LIM-01, TH-P2-INV-04
Estimate: 1d

Objective: Allow enterprise owner/admin users to set member monthly spend limits.

Current State: No API exists for enterprise member monthly limits.

Scope: Create read/update APIs for limit amount, currency, effective month, and disabled/unlimited state.

Out of Scope: Gateway enforcement and frontend UI.

Implementation Notes: Audit every limit mutation.

Acceptance Criteria:

- AC-01: Given an owner sets member limit to 100.00 for current month, read API returns 100.00 for that member and month.
- AC-02: Given a regular member calls update limit, API returns 403 and stored limit remains unchanged.
- AC-03: Given the target member is not in the tenant, API returns 404.
- AC-04: Given limit amount is negative, API returns 400 and no row is changed.

Test Requirements: Unit validation tests; integration owner/admin/member permission tests; regression for membership APIs.

Observability Requirements: Emit limit mutation counter by role and result.

Audit Requirements: Write `enterprise_member_limit.updated` with operator, target, before value, and after value.

Migration / Rollback Requirements: No migration beyond TH-P2-LIM-01. Rollback disables limit APIs.

Documentation Requirements: Document limit payload and permission rules.

Risks: Incorrect role checks can let members raise their own limits.

Definition of Done: Global DoD applies.

### TH-P2-LIM-03

Task ID: TH-P2-LIM-03
Title: Redis Monthly Counter
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-LIM-01
Estimate: 1.5d

Objective: Reserve member monthly limit amount in Redis before upstream gateway execution.

Current State: No per-member monthly spend counter exists.

Scope: Create atomic Redis reservation for tenant, member, month, currency, and amount.

Out of Scope: DB fallback and final charge correction.

Implementation Notes: Use atomic script or transaction so concurrent requests cannot overspend the same limit.

Acceptance Criteria:

- AC-01: Given remaining monthly limit is 10.00 and request reserves 6.00, Redis counter records a 6.00 pending amount.
- AC-02: Given remaining monthly limit is 5.00 and request reserves 6.00, reservation returns limit exceeded and no counter changes.
- AC-03: Given two concurrent 6.00 reservations race against remaining 10.00, at most one reservation succeeds.

Test Requirements: Unit key/amount tests; integration Redis atomic reservation tests; concurrency test for competing reservations.

Observability Requirements: Emit monthly limit reserve success and reject metrics.

Audit Requirements: No audit for successful reserve; denied limit events are audited in gateway tasks.

Migration / Rollback Requirements: No migration. Rollback disables Redis counter caller.

Documentation Requirements: Document Redis key format and TTL.

Risks: Non-atomic counters allow concurrent monthly limit overspend.

Definition of Done: Global DoD applies.

### TH-P2-LIM-04

Task ID: TH-P2-LIM-04
Title: DB Usage Fallback
Phase: P2
Epic: Enterprise
Type: Gateway / Reliability
Priority: P1
Dependencies: TH-P2-LIM-03
Estimate: 1.5d

Objective: Calculate monthly remaining spend from DB when Redis is unavailable.

Current State: No DB fallback path exists for monthly limit enforcement.

Scope: Query current month usage and pending reservations from DB and make a deterministic allow/deny decision.

Out of Scope: Fail-closed policy when DB also fails and final counter correction.

Implementation Notes: Fallback must be slower but financially safe.

Acceptance Criteria:

- AC-01: Given Redis is unavailable and DB shows 4.00 used of a 10.00 limit, a 5.00 request is allowed.
- AC-02: Given Redis is unavailable and DB shows 8.00 used of a 10.00 limit, a 3.00 request is denied.
- AC-03: Given DB query fails during fallback, fallback returns an explicit error without allowing the request.

Test Requirements: Unit remaining amount tests; integration fallback DB query; failure injection for Redis unavailable and DB error.

Observability Requirements: Emit Redis fallback counter and fallback DB error counter.

Audit Requirements: Audit denied fallback decisions with member id and amount, no prompt content.

Migration / Rollback Requirements: No migration. Rollback disables fallback path.

Documentation Requirements: Document fallback decision formula.

Risks: Ignoring pending reservations in DB fallback can exceed limits.

Definition of Done: Global DoD applies.

### TH-P2-LIM-05

Task ID: TH-P2-LIM-05
Title: Limit Fail-Closed Policy
Phase: P2
Epic: Enterprise
Type: Gateway / Safety
Priority: P1
Dependencies: TH-P2-LIM-04
Estimate: 1d

Objective: Reject enterprise gateway requests when monthly limit state cannot be determined.

Current State: Fallback can return an error, but gateway policy is not defined.

Scope: Map Redis and DB failure combinations to explicit gateway responses and audit events.

Out of Scope: Counter implementation and UI messaging.

Implementation Notes: Do not call upstream when both Redis and DB cannot verify limit availability.

Acceptance Criteria:

- AC-01: Given Redis is available, gateway uses Redis reservation result.
- AC-02: Given Redis is unavailable and DB fallback succeeds, gateway uses DB fallback result.
- AC-03: Given Redis is unavailable and DB fallback fails, gateway returns 503 and upstream is not called.
- AC-04: Given limit is exceeded, gateway returns 429 or configured billing-limit status and upstream is not called.

Test Requirements: Unit failure policy matrix; integration gateway response tests; failure injection for Redis outage plus DB outage.

Observability Requirements: Emit fail-closed counter by dependency failure.

Audit Requirements: Write `enterprise_limit.denied` for exceeded and indeterminate limit decisions.

Migration / Rollback Requirements: No migration. Rollback removes fail-closed monthly limit gate.

Documentation Requirements: Document response codes for limit failures.

Risks: Fail-open behavior can send provider traffic with no enforceable member budget.

Definition of Done: Global DoD applies.

### TH-P2-LIM-06

Task ID: TH-P2-LIM-06
Title: Concurrent Limit Reservation
Phase: P2
Epic: Enterprise
Type: Test / Finance
Priority: P1
Dependencies: TH-P2-LIM-05
Estimate: 1.5d

Objective: Prove concurrent gateway requests cannot exceed member monthly limit.

Current State: No concurrency test exists for monthly limit enforcement.

Scope: Add integration tests for Redis path and fallback path concurrency.

Out of Scope: UI and final counter correction.

Implementation Notes: Use deterministic amounts such as two 6.00 requests against 10.00 remaining.

Acceptance Criteria:

- AC-01: Given two concurrent 6.00 requests compete against 10.00 remaining in Redis path, no more than one request reaches upstream.
- AC-02: Given Redis is unavailable and fallback lock is used, two concurrent 6.00 requests against 10.00 remaining do not both reach upstream.
- AC-03: Given one request is denied by limit, no wallet reserve is created for that denied request.

Test Requirements: Integration gateway/counter tests; regression single-request path; concurrency required for Redis and DB fallback paths.

Observability Requirements: Tests assert denied/concurrent metrics are emitted where metrics hooks exist.

Audit Requirements: Tests assert denied request audit event exists.

Migration / Rollback Requirements: No migration. Rollback removes tests only.

Documentation Requirements: Document concurrency scenario in test notes.

Risks: Missing fallback locking can pass Redis tests but fail during outage.

Definition of Done: Global DoD applies.

### TH-P2-LIM-07

Task ID: TH-P2-LIM-07
Title: Final Charge Counter Correction
Phase: P2
Epic: Enterprise
Type: Gateway / Finance
Priority: P1
Dependencies: TH-P2-LIM-06, TH-P2-GW-07
Estimate: 1.5d

Objective: Correct monthly counters from reserved amount to actual final charge after gateway completion.

Current State: Monthly limit reservation is planned separately from actual usage settlement.

Scope: Adjust counters on settle, release counters on failure, and preserve idempotency.

Out of Scope: Statement UI and Smart Routing.

Implementation Notes: Final counter correction must align with enterprise wallet settle/release result.

Acceptance Criteria:

- AC-01: Given 10.00 is reserved and final charge is 7.00, monthly used amount increases by 7.00 and 3.00 pending amount is cleared.
- AC-02: Given upstream fails before billable usage, monthly pending amount is released and used amount is unchanged.
- AC-03: Given final correction is retried with the same request id, monthly counters change once.
- AC-04: Given counter correction fails after wallet settle, request is marked for reconciliation review and no duplicate wallet debit is made.

Test Requirements: Unit correction matrix; integration gateway settle/release plus counter correction; failure injection after wallet settle.

Observability Requirements: Emit counter correction success, retry, and failure metrics.

Audit Requirements: Audit correction failure with request id and tenant/member ids.

Migration / Rollback Requirements: No migration. Rollback disables monthly limit enforcement while preserving wallet charging.

Documentation Requirements: Document counter lifecycle.

Risks: Counter correction failure can block future legitimate requests if not visible.

Definition of Done: Global DoD applies.

## Enterprise Usage And UI

### TH-P2-UI-01

Task ID: TH-P2-UI-01
Title: Enterprise Usage Query API
Phase: P2
Epic: Enterprise
Type: Backend / Reporting
Priority: P2
Dependencies: TH-P2-GW-07
Estimate: 1.5d

Objective: Expose enterprise usage records filtered by tenant, member, model, endpoint, and date range.

Current State: Personal usage APIs exist; enterprise-scoped usage API is missing.

Scope: Create paginated enterprise usage read API with permission checks.

Out of Scope: Statement aggregation and frontend.

Implementation Notes: Removed members remain visible in historical records to owner/admin roles.

Acceptance Criteria:

- AC-01: Given an owner queries current month usage, response contains only rows for that tenant.
- AC-02: Given a regular member queries usage, response contains only their own rows unless product permission grants broader read.
- AC-03: Given an unrelated user requests a tenant id, API returns 403 or 404 and no rows.

Test Requirements: Unit filter/permission tests; integration tenant/member scoped query; regression for personal usage API.

Observability Requirements: Emit usage query counter by role and result.

Audit Requirements: Audit forbidden usage query attempts.

Migration / Rollback Requirements: No migration. Rollback disables enterprise usage API.

Documentation Requirements: Document filters and role visibility.

Risks: Cross-tenant leakage is a severe data privacy defect.

Definition of Done: Global DoD applies.

### TH-P2-UI-02

Task ID: TH-P2-UI-02
Title: Enterprise Statement API
Phase: P2
Epic: Enterprise
Type: Backend / Reporting
Priority: P2
Dependencies: TH-P2-UI-01
Estimate: 1.5d

Objective: Provide monthly enterprise statement totals from usage and wallet ledger.

Current State: Personal billing statements exist; enterprise statement endpoint is missing.

Scope: Return monthly spend, member breakdown, model breakdown, wallet debit total, and reconciliation flag.

Out of Scope: PDF export and frontend.

Implementation Notes: Statement totals must reconcile to ledger totals for the same period.

Acceptance Criteria:

- AC-01: Given seeded enterprise usage and wallet ledger for one month, statement total equals ledger debit total.
- AC-02: Given usage exists for two tenants, tenant A statement excludes tenant B rows.
- AC-03: Given ledger and usage totals differ, response includes a reconciliation flag and does not rewrite either source.

Test Requirements: Unit aggregation math tests; integration seeded usage/ledger statement; failure injection for DB error.

Observability Requirements: Emit statement query and mismatch counters.

Audit Requirements: Audit forbidden statement access attempts.

Migration / Rollback Requirements: No migration. Rollback disables statement API.

Documentation Requirements: Document total formulas.

Risks: Statement numbers that do not tie to ledger will erode billing trust.

Definition of Done: Global DoD applies.

### TH-P2-UI-03

Task ID: TH-P2-UI-03
Title: Enterprise Wallet View API
Phase: P2
Epic: Enterprise
Type: Backend / API
Priority: P2
Dependencies: TH-P2-WAL-08
Estimate: 1d

Objective: Expose enterprise wallet summary data needed by console UI.

Current State: Wallet transaction listing is available as a backend slice, but UI summary contract is not separated.

Scope: Return balance, available balance, pending reserve total, recent ledger entries, and low balance state.

Out of Scope: Top-up payment flow and frontend rendering.

Implementation Notes: Reuse wallet listing authorization from TH-P2-WAL-08.

Acceptance Criteria:

- AC-01: Given an owner requests wallet summary, response includes balance, available balance, pending reserve total, and recent entries.
- AC-02: Given a regular member requests wallet summary, API returns 403.
- AC-03: Given wallet does not exist for an active tenant, API creates or returns a safe zero-balance wallet according to TH-P2-WAL-03 behavior.

Test Requirements: Unit summary DTO tests; integration permission and wallet state cases; regression for wallet listing API.

Observability Requirements: Emit wallet summary read counter.

Audit Requirements: Audit forbidden wallet summary access.

Migration / Rollback Requirements: No migration. Rollback disables summary route.

Documentation Requirements: Document summary response fields.

Risks: Displaying balance without pending reserves can mislead enterprise owners.

Definition of Done: Global DoD applies.

### TH-P2-UI-04

Task ID: TH-P2-UI-04
Title: Enterprise Center Route Shell
Phase: P2
Epic: Enterprise
Type: Frontend / Routing
Priority: P2
Dependencies: TH-P2-04
Estimate: 1d

Objective: Create the enterprise center route shell with status-aware views.

Current State: Console UI does not have a complete enterprise center route.

Scope: Add route, layout shell, pending/rejected/active states, and navigation entry visibility.

Out of Scope: Members, wallet, usage, and statement panels.

Implementation Notes: Use existing frontend components and design patterns.

Acceptance Criteria:

- AC-01: Given user has pending application, enterprise route displays pending status view.
- AC-02: Given user has active membership, enterprise route displays active shell tabs.
- AC-03: Given user has no enterprise relationship, route displays application entry state.

Test Requirements: Unit route state mapper tests; integration mocked API states; manual visual check for each state.

Observability Requirements: No new runtime metric required.

Audit Requirements: No audit for frontend-only route rendering.

Migration / Rollback Requirements: No migration. Rollback removes route and nav entry.

Documentation Requirements: Document route states.

Risks: Route shell that assumes active tenant can hide review failures.

Definition of Done: Global DoD applies.

### TH-P2-UI-05

Task ID: TH-P2-UI-05
Title: Enterprise Members UI
Phase: P2
Epic: Enterprise
Type: Frontend / UI
Priority: P2
Dependencies: TH-P2-INV-03, TH-P2-INV-05, TH-P2-INV-06, TH-P2-LIM-02
Estimate: 1.5d

Objective: Build enterprise member management UI using existing API slices.

Current State: Member APIs are planned; UI is missing.

Scope: Render member list, invite creation, invitation revoke, role update, member removal, and monthly limit edit controls.

Out of Scope: Wallet and usage UI.

Implementation Notes: Hide or disable controls based on role returned by API; backend remains the authority.

Acceptance Criteria:

- AC-01: Given owner role, UI renders invite, role update, removal, and limit edit controls.
- AC-02: Given member role, UI does not render management controls and API 403 responses show a non-sensitive error state.
- AC-03: Given owner attempts last-owner demotion, UI shows backend 409 response and keeps displayed role unchanged after refresh.

Test Requirements: Unit permission rendering tests; integration mocked API responses; E2E/manual owner/admin/member walkthrough.

Observability Requirements: No new runtime metric required.

Audit Requirements: Frontend must surface backend-audited action ids when present.

Migration / Rollback Requirements: No migration. Rollback hides member panel route.

Documentation Requirements: Document UI permission behavior.

Risks: Frontend-only permission checks can mislead users if backend is not authoritative.

Definition of Done: Global DoD applies.

### TH-P2-UI-06

Task ID: TH-P2-UI-06
Title: Enterprise Wallet UI
Phase: P2
Epic: Enterprise
Type: Frontend / UI
Priority: P2
Dependencies: TH-P2-UI-03
Estimate: 1d

Objective: Build enterprise wallet summary and ledger UI.

Current State: Enterprise wallet APIs are planned; UI is missing.

Scope: Render balance, available balance, pending reserves, low balance state, and recent transactions.

Out of Scope: Top-up payment provider flow.

Implementation Notes: Use existing wallet display conventions where possible.

Acceptance Criteria:

- AC-01: Given wallet summary API returns balance and pending reserve total, UI renders both numbers exactly as returned.
- AC-02: Given API returns 403, UI renders forbidden state without showing stale wallet values.
- AC-03: Given recent transactions are paginated, UI loads the next page without duplicating rows.

Test Requirements: Unit money formatting tests; integration mocked wallet API states; manual visual check with balance, zero, and forbidden states.

Observability Requirements: No new runtime metric required.

Audit Requirements: No frontend audit beyond backend actions.

Migration / Rollback Requirements: No migration. Rollback hides wallet panel.

Documentation Requirements: Document wallet UI states.

Risks: Stale balance rendering can cause wrong financial decisions.

Definition of Done: Global DoD applies.

### TH-P2-UI-07

Task ID: TH-P2-UI-07
Title: Enterprise Usage Statement UI
Phase: P2
Epic: Enterprise
Type: Frontend / UI
Priority: P2
Dependencies: TH-P2-UI-01, TH-P2-UI-02
Estimate: 1.5d

Objective: Build enterprise usage and statement UI from enterprise reporting APIs.

Current State: Enterprise reporting APIs are planned; UI is missing.

Scope: Render monthly totals, member/model breakdowns, usage table, filters, and reconciliation flag.

Out of Scope: PDF export and advanced analytics.

Implementation Notes: Do not hide reconciliation mismatch flags behind collapsed-only UI.

Acceptance Criteria:

- AC-01: Given statement API returns monthly totals, UI renders the exact amount and currency from response.
- AC-02: Given usage API returns two pages, pagination displays both without cross-tenant rows.
- AC-03: Given statement response has reconciliation flag, UI renders a visible mismatch state.

Test Requirements: Unit table/chart mapping tests; integration mocked reporting responses; E2E/manual date filter and pagination walkthrough.

Observability Requirements: No new runtime metric required.

Audit Requirements: No frontend audit beyond backend reads.

Migration / Rollback Requirements: No migration. Rollback hides reporting panel.

Documentation Requirements: Document reporting UI states.

Risks: Mismatch flags hidden from operators delay finance review.

Definition of Done: Global DoD applies.

### TH-P2-UI-08

Task ID: TH-P2-UI-08
Title: Enterprise Access E2E Matrix
Phase: P2
Epic: Enterprise
Type: Test / E2E
Priority: P2
Dependencies: TH-P2-UI-05, TH-P2-UI-06, TH-P2-UI-07
Estimate: 1.5d

Objective: Verify the end-to-end enterprise console for applicant, owner, admin, member, removed member, and unrelated user.

Current State: No enterprise console E2E matrix exists.

Scope: Create browser or API-backed E2E scenarios across enterprise status and role combinations.

Out of Scope: Provider payment tests and RBAC admin tests.

Implementation Notes: Use stable test fixtures and avoid depending on production data.

Acceptance Criteria:

- AC-01: Given applicant is pending, E2E verifies pending view and absence of wallet/member management controls.
- AC-02: Given owner/admin/member roles, E2E verifies each role sees only its allowed enterprise controls.
- AC-03: Given removed member logs in, E2E verifies enterprise gateway and console management access are denied.
- AC-04: Given unrelated user opens enterprise route, E2E verifies no other tenant data is visible.

Test Requirements: Integration seeded role fixtures; regression console smoke; E2E/manual required for all listed roles.

Observability Requirements: No runtime metric required.

Audit Requirements: E2E asserts forbidden backend actions generate audit events.

Migration / Rollback Requirements: No migration. Rollback removes E2E specs only.

Documentation Requirements: Document fixture setup.

Risks: Without a matrix, role regressions can hide behind happy-path owner tests.

Definition of Done: Global DoD applies.

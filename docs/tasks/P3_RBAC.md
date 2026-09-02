# P3 RBAC Sprint Tasks

## Epic Code Audit

Existing:

- `users.role` currently distinguishes only `user` and `admin`.
- `AdminAuth()` checks whether context role equals `admin`.
- Admin route groups already use console auth and admin auth middleware.
- Frontend navigation uses binary admin gating.

Partial:

- Audit middleware and admin route tests exist, but they do not model granular permissions.
- The enterprise plan document describes `admin_roles`, `admin_role_permissions`, and `users.admin_role_id`, but code does not yet implement them.

Missing:

- Permission catalog source of truth.
- Role schema, repository, CRUD API, assignment API, and route-level `RequirePerm`.
- Super Admin protection and self-lockout guards.
- Frontend permission hydration, navigation filtering, action gating, and role management UI.

Risks:

- Binary admin access is too broad for production operations.
- Route-by-route conversion can accidentally leave endpoints unguarded.
- Role assignment without self-lockout rules can remove the last Super Admin.

## Split Rule

Every RBAC task is sized at 0.5d-2d and can be independently developed, reviewed, tested, accepted, and rolled back.

### TH-P3-01

Task ID: TH-P3-01
Title: Permission Catalog Definition
Phase: P3
Epic: RBAC
Type: Backend / Authorization
Priority: P1
Dependencies: TH-P05-09
Estimate: 1d

Objective: Define the canonical permission catalog used by backend and frontend.

Current State: Permissions are implicit in route code and binary admin checks.

Scope: Create permission names, descriptions, grouping, and route/action mapping table.

Out of Scope: Database migration and runtime enforcement.

Implementation Notes: Keep catalog deterministic and reject unknown permission strings in later tasks.

Acceptance Criteria:

- AC-01: Given the catalog is loaded, it includes permissions for users, roles, tenants, payments, wallets, models, channels, audit, and system settings.
- AC-02: Given a permission string is not in the catalog, validation returns an unknown-permission error.
- AC-03: Given an admin route matrix is generated from the catalog, every current admin route has an explicit permission or Super Admin-only decision.

Test Requirements: Unit catalog validation tests; regression route inventory comparison; manual security review.

Observability Requirements: No runtime metric required.

Audit Requirements: Catalog must list audit event names for future mutations.

Migration / Rollback Requirements: No migration. Rollback removes catalog definitions.

Documentation Requirements: Document permission naming convention and route matrix.

Risks: Missing route mapping can preserve all-or-nothing admin behavior.

Definition of Done: Global DoD applies.

### TH-P3-02

Task ID: TH-P3-02
Title: RBAC Migration And Seed Roles
Phase: P3
Epic: RBAC
Type: Backend / Migration
Priority: P1
Dependencies: TH-P3-01
Estimate: 1.5d

Objective: Add role and role-permission schema and seed Super Admin from existing admins.

Current State: No RBAC tables or `users.admin_role_id` column exist.

Scope: Add `admin_roles`, `admin_role_permissions`, `users.admin_role_id`, built-in Super Admin role, and data migration for existing `users.role='admin'`.

Out of Scope: CRUD APIs and middleware enforcement.

Implementation Notes: Existing admins must keep admin access after migration.

Acceptance Criteria:

- AC-01: Given migration runs on a DB with two `admin` users, both users reference the seeded Super Admin role.
- AC-02: Given migration rollback runs, RBAC tables and column are removed and existing `users.role` values remain.
- AC-03: Given a seeded Super Admin role is queried, it contains every permission from TH-P3-01.

Test Requirements: Integration migration up/down tests; regression existing admin auth tests; failure injection for partial migration failure.

Observability Requirements: No runtime metric required.

Audit Requirements: Migration notes must record seeded role behavior.

Migration / Rollback Requirements: Migration and rollback scripts are required and verified.

Documentation Requirements: Update deployment notes for admin role seed.

Risks: Bad data migration can lock all admins out.

Definition of Done: Global DoD applies.

### TH-P3-03

Task ID: TH-P3-03
Title: Role Repository
Phase: P3
Epic: RBAC
Type: Backend / Repository
Priority: P1
Dependencies: TH-P3-02
Estimate: 1d

Objective: Implement repository methods for admin roles and permission assignments.

Current State: No role repository exists.

Scope: Add list, find, create, update, assign permissions, and user role lookup methods.

Out of Scope: HTTP handlers and frontend.

Implementation Notes: Repository must reject unknown permissions through the catalog.

Acceptance Criteria:

- AC-01: Given a role is created with known permissions, repository returns the role with matching permissions.
- AC-02: Given a role is created with an unknown permission, repository returns validation error and no role is inserted.
- AC-03: Given DB write fails during permission assignment, role and permissions roll back together.

Test Requirements: Unit error mapping tests; integration repository CRUD tests; failure injection for permission insert failure.

Observability Requirements: No runtime metric required.

Audit Requirements: Repository does not write audit events directly.

Migration / Rollback Requirements: No migration beyond TH-P3-02. Rollback removes repository usage.

Documentation Requirements: Document repository invariants.

Risks: Orphan permissions can make route behavior unpredictable.

Definition of Done: Global DoD applies.

### TH-P3-04

Task ID: TH-P3-04
Title: Role List Create API
Phase: P3
Epic: RBAC
Type: Backend / Admin API
Priority: P1
Dependencies: TH-P3-03
Estimate: 1d

Objective: Expose role list and create APIs for authorized administrators.

Current State: Role repository is planned; no HTTP API exists.

Scope: Implement list roles and create role with permission set.

Out of Scope: Update, delete, assignment, and frontend.

Implementation Notes: Require Super Admin or `roles:write` according to catalog decision.

Acceptance Criteria:

- AC-01: Given Super Admin creates a role with valid permissions, API returns 201 and persisted role id.
- AC-02: Given admin without `roles:write` creates a role, API returns 403 and no role is created.
- AC-03: Given unauthenticated caller lists roles, API returns 401.
- AC-04: Given role name is duplicate, API returns 409.

Test Requirements: Unit request validation tests; integration unauthenticated/no-permission/allowed/Super Admin cases.

Observability Requirements: Emit role create counter by result.

Audit Requirements: Write `admin_role.created` with operator, role id, and permission count.

Migration / Rollback Requirements: No migration. Rollback removes routes.

Documentation Requirements: Document role create payload.

Risks: Missing create audit weakens permission change traceability.

Definition of Done: Global DoD applies.

### TH-P3-05

Task ID: TH-P3-05
Title: Role Detail Update API
Phase: P3
Epic: RBAC
Type: Backend / Admin API
Priority: P1
Dependencies: TH-P3-04
Estimate: 1.5d

Objective: Expose role detail and update APIs with permission validation.

Current State: Role create/list is separate; update path is missing.

Scope: Read role detail, update name/description/permissions, and return before/after state.

Out of Scope: Delete and user assignment.

Implementation Notes: Built-in Super Admin role permissions must not be reduced.

Acceptance Criteria:

- AC-01: Given Super Admin updates a custom role permission set, response returns the new permission list.
- AC-02: Given admin without `roles:write` updates a role, API returns 403 and role remains unchanged.
- AC-03: Given update payload includes unknown permission, API returns 400 and role remains unchanged.
- AC-04: Given request reduces built-in Super Admin permissions, API returns 409.

Test Requirements: Unit validation tests; integration permission and immutable Super Admin tests; regression create/list APIs.

Observability Requirements: Emit role update counter by result.

Audit Requirements: Write `admin_role.updated` with operator, role id, before permissions, and after permissions.

Migration / Rollback Requirements: No migration. Rollback disables update route.

Documentation Requirements: Document immutable built-in role behavior.

Risks: Reducing Super Admin permissions can create irreversible lockout.

Definition of Done: Global DoD applies.

### TH-P3-06

Task ID: TH-P3-06
Title: Role Delete Guard
Phase: P3
Epic: RBAC
Type: Backend / Admin API
Priority: P1
Dependencies: TH-P3-04
Estimate: 1d

Objective: Allow deletion of unused custom roles while preventing unsafe deletes.

Current State: Role delete API is missing.

Scope: Delete custom roles only when no users are assigned and role is not built-in.

Out of Scope: Role update and assignment.

Implementation Notes: Return clear conflict errors for assigned roles.

Acceptance Criteria:

- AC-01: Given an unused custom role is deleted by Super Admin, subsequent detail lookup returns 404.
- AC-02: Given a role has at least one assigned user, delete returns 409 and role remains.
- AC-03: Given built-in Super Admin role is deleted, API returns 409 and role remains.
- AC-04: Given unauthenticated request calls delete, API returns 401.

Test Requirements: Unit delete guard tests; integration assigned/built-in/unauthenticated cases.

Observability Requirements: Emit role delete counter by result.

Audit Requirements: Write `admin_role.deleted` for successful delete and forbidden/conflict audit for blocked destructive attempts.

Migration / Rollback Requirements: No migration. Rollback disables delete route.

Documentation Requirements: Document delete constraints.

Risks: Deleting assigned roles can leave admins with undefined permissions.

Definition of Done: Global DoD applies.

### TH-P3-07

Task ID: TH-P3-07
Title: User Role Assignment Guard
Phase: P3
Epic: RBAC
Type: Backend / Admin API
Priority: P1
Dependencies: TH-P3-03
Estimate: 1.5d

Objective: Assign admin roles to users without self-lockout or last-Super-Admin loss.

Current State: User role assignment is binary and no admin role assignment API exists.

Scope: Assign, change, and remove `admin_role_id` under permission and self-protection rules.

Out of Scope: Role CRUD and frontend.

Implementation Notes: DB role state must override stale JWT role claims.

Acceptance Criteria:

- AC-01: Given Super Admin assigns a custom role to an admin user, subsequent permission lookup returns the custom role permissions.
- AC-02: Given admin without user-role assignment permission calls the API, it returns 403 and target user remains unchanged.
- AC-03: Given the only Super Admin tries to remove their own Super Admin role, API returns 409 and role remains.
- AC-04: Given two concurrent requests try to demote the last two Super Admin users, at least one Super Admin remains after both complete.

Test Requirements: Unit self-lockout matrix; integration assignment cases; concurrency test for last-Super-Admin guard.

Observability Requirements: Emit assignment counter by result and target role type.

Audit Requirements: Write `admin_user.role_assigned` with operator, target, before role, and after role.

Migration / Rollback Requirements: No migration. Rollback disables assignment route and keeps seeded admin role data.

Documentation Requirements: Document self-lockout protection.

Risks: Assignment bugs can remove production administrative access.

Definition of Done: Global DoD applies.

### TH-P3-08

Task ID: TH-P3-08
Title: Current User Permissions API
Phase: P3
Epic: RBAC
Type: Backend / API
Priority: P1
Dependencies: TH-P3-07
Estimate: 1d

Objective: Return the current admin user's effective permissions to frontend and middleware tests.

Current State: Frontend only knows binary admin state.

Scope: Add `GET /me/permissions` or equivalent console endpoint returning role id, role name, and permissions.

Out of Scope: Route enforcement and frontend rendering.

Implementation Notes: Super Admin returns full catalog; normal users return no admin permissions.

Acceptance Criteria:

- AC-01: Given Super Admin calls the endpoint, response includes every permission from catalog.
- AC-02: Given admin with custom role calls the endpoint, response includes exactly that role's permissions.
- AC-03: Given unauthenticated request calls the endpoint, API returns 401.
- AC-04: Given regular user calls the endpoint, response has no admin permissions and no admin role id.

Test Requirements: Unit permission aggregation tests; integration Super Admin/custom role/regular user/unauthenticated cases.

Observability Requirements: Emit permission lookup counter by result.

Audit Requirements: Audit forbidden lookups only.

Migration / Rollback Requirements: No migration. Rollback disables endpoint.

Documentation Requirements: Document response schema for frontend.

Risks: Returning stale JWT permissions can keep demoted admins authorized.

Definition of Done: Global DoD applies.

### TH-P3-09

Task ID: TH-P3-09
Title: RequirePerm Middleware
Phase: P3
Epic: RBAC
Type: Backend / Authorization
Priority: P1
Dependencies: TH-P3-08
Estimate: 1.5d

Objective: Enforce route permissions through middleware while preserving Super Admin access.

Current State: `AdminAuth()` only checks binary admin role.

Scope: Implement `RequirePerm(permission)` with unauthenticated, no permission, allowed, Super Admin, and stale JWT behavior.

Out of Scope: Converting every route and frontend.

Implementation Notes: Middleware must read live DB-derived permissions when existing auth pipeline refreshes role state.

Acceptance Criteria:

- AC-01: Given unauthenticated request reaches a protected route, middleware returns 401.
- AC-02: Given authenticated admin lacks required permission, middleware returns 403.
- AC-03: Given authenticated admin has required permission, request reaches handler.
- AC-04: Given Super Admin calls any `RequirePerm` route, request reaches handler.
- AC-05: Given token says admin but DB role was demoted, middleware returns 403 for missing permission.

Test Requirements: Unit middleware decision tests; integration route wrapper tests; regression for existing `AdminAuth()` behavior until route conversion.

Observability Requirements: Emit authorization deny counter by permission and reason.

Audit Requirements: Write `authorization.denied` with user, permission, route, and reason.

Migration / Rollback Requirements: No migration. Rollback leaves routes on `AdminAuth()`.

Documentation Requirements: Document middleware usage examples.

Risks: Fail-open middleware would expose admin operations.

Definition of Done: Global DoD applies.

### TH-P3-10

Task ID: TH-P3-10
Title: Admin Route Permission Matrix
Phase: P3
Epic: RBAC
Type: Backend / Authorization
Priority: P1
Dependencies: TH-P3-09
Estimate: 1.5d

Objective: Convert admin routes from binary admin checks to explicit permission checks.

Current State: Admin route groups rely on `AdminAuth()`.

Scope: Apply `RequirePerm` to selected admin route groups according to the catalog matrix.

Out of Scope: Frontend UI gating and new role CRUD.

Implementation Notes: Convert in small route groups and keep regression tests for critical operations.

Acceptance Criteria:

- AC-01: Given each converted route is listed in the matrix, the route has exactly one required permission or Super Admin-only marker.
- AC-02: Given admin lacks a converted route permission, route returns 403 and handler side effects do not run.
- AC-03: Given admin has the converted route permission, route returns the handler response.
- AC-04: Given Super Admin calls converted routes, routes remain accessible.

Test Requirements: Unit route matrix validation; integration permission cases for each converted group; regression admin route smoke.

Observability Requirements: Route denies use middleware deny metric.

Audit Requirements: Route denies use middleware audit event.

Migration / Rollback Requirements: No migration. Rollback restores route group to `AdminAuth()`.

Documentation Requirements: Update route permission matrix.

Risks: Partial conversion can leave sensitive routes overexposed.

Definition of Done: Global DoD applies.

### TH-P3-11

Task ID: TH-P3-11
Title: Authorization Failure Audit Event
Phase: P3
Epic: RBAC
Type: Audit / Security
Priority: P1
Dependencies: TH-P3-09
Estimate: 1d

Objective: Standardize audit entries for permission denials.

Current State: Audit middleware exists, but RBAC denial payload is not standardized.

Scope: Add structured audit payload for permission, route, user, role, reason, and request id.

Out of Scope: SIEM export and UI audit viewer changes.

Implementation Notes: Do not log request bodies or tokens.

Acceptance Criteria:

- AC-01: Given a no-permission request is denied, audit event contains user id, route, permission, reason, and request id.
- AC-02: Given an unauthenticated request is denied, audit event omits user id and includes route and reason.
- AC-03: Given request contains Authorization header, audit payload does not contain the token value.

Test Requirements: Unit audit payload tests; integration denial audit tests; regression existing audit events remain readable.

Observability Requirements: Emit audit write failure counter.

Audit Requirements: This task defines the RBAC denial audit event.

Migration / Rollback Requirements: No migration unless audit event enum requires one.

Documentation Requirements: Document denial audit schema.

Risks: Logging tokens in denied requests is a credential leak.

Definition of Done: Global DoD applies.

### TH-P3-12

Task ID: TH-P3-12
Title: Frontend Permission Hydration
Phase: P3
Epic: RBAC
Type: Frontend / State
Priority: P2
Dependencies: TH-P3-08
Estimate: 1d

Objective: Load current user permissions into frontend state.

Current State: Frontend uses binary admin checks.

Scope: Fetch permission endpoint, cache permissions in auth state, and refresh after login or role change.

Out of Scope: Navigation filtering and action gating.

Implementation Notes: Empty permission list is a valid state for regular users.

Acceptance Criteria:

- AC-01: Given Super Admin logs in, frontend state contains full permission list returned by API.
- AC-02: Given regular user logs in, frontend state contains an empty admin permission list.
- AC-03: Given permission endpoint returns 401, frontend clears permission state.

Test Requirements: Unit state mapper tests; integration mocked API cases; regression existing login flow.

Observability Requirements: No new runtime metric required.

Audit Requirements: No frontend audit event required.

Migration / Rollback Requirements: No migration. Rollback removes permission state usage.

Documentation Requirements: Document frontend permission state shape.

Risks: Stale permission state can show unavailable admin actions.

Definition of Done: Global DoD applies.

### TH-P3-13

Task ID: TH-P3-13
Title: Admin Navigation Permission Filtering
Phase: P3
Epic: RBAC
Type: Frontend / Navigation
Priority: P2
Dependencies: TH-P3-12
Estimate: 1d

Objective: Filter admin navigation items based on hydrated permissions.

Current State: Navigation uses binary admin visibility.

Scope: Map nav items to permissions and hide items when permission is absent.

Out of Scope: Button-level gating and backend enforcement.

Implementation Notes: Backend remains authoritative; hidden nav is a UX layer.

Acceptance Criteria:

- AC-01: Given user lacks `payments:read`, payment admin nav item is not rendered.
- AC-02: Given user has `payments:read`, payment admin nav item is rendered.
- AC-03: Given Super Admin logs in, all admin nav items from the matrix are rendered.

Test Requirements: Unit nav filtering tests; integration mocked permission states; visual regression for admin nav layout.

Observability Requirements: No runtime metric required.

Audit Requirements: No frontend audit event required.

Migration / Rollback Requirements: No migration. Rollback restores binary admin nav gating.

Documentation Requirements: Document nav permission map.

Risks: Incorrect nav hiding can make authorized workflows hard to find.

Definition of Done: Global DoD applies.

### TH-P3-14

Task ID: TH-P3-14
Title: Admin Action Permission Gating
Phase: P3
Epic: RBAC
Type: Frontend / UI
Priority: P2
Dependencies: TH-P3-12
Estimate: 1.5d

Objective: Gate sensitive admin buttons and actions using hydrated permissions.

Current State: Action buttons generally assume binary admin access.

Scope: Apply permission checks to create/update/delete/complete actions in selected admin pages.

Out of Scope: Backend enforcement and role management UI.

Implementation Notes: Hide or disable controls consistently and surface backend 403 errors.

Acceptance Criteria:

- AC-01: Given user lacks required write permission, protected action button is not clickable or not rendered.
- AC-02: Given user has required write permission, protected action button performs the existing API action.
- AC-03: Given backend returns 403 after stale frontend permission state, UI shows a forbidden state and refreshes permissions.

Test Requirements: Unit action visibility tests; integration mocked 403 and success responses; regression existing admin pages.

Observability Requirements: No new runtime metric required.

Audit Requirements: No frontend audit beyond backend actions.

Migration / Rollback Requirements: No migration. Rollback removes frontend action gating.

Documentation Requirements: Document action permission map.

Risks: Frontend gating without backend enforcement is not sufficient; dependency on TH-P3-09 remains mandatory.

Definition of Done: Global DoD applies.

### TH-P3-15

Task ID: TH-P3-15
Title: Role Management UI
Phase: P3
Epic: RBAC
Type: Frontend / UI
Priority: P2
Dependencies: TH-P3-05, TH-P3-06, TH-P3-12
Estimate: 1.5d

Objective: Build UI for listing, creating, editing, and deleting admin roles.

Current State: No RBAC role management UI exists.

Scope: Render role table, detail form, permission checklist, create/update/delete flows, and conflict states.

Out of Scope: User assignment UI unless included in an existing admin user page.

Implementation Notes: Built-in Super Admin role must appear read-only.

Acceptance Criteria:

- AC-01: Given Super Admin opens role management, UI lists roles and permission counts.
- AC-02: Given custom role edit succeeds, UI refreshes and displays updated permissions.
- AC-03: Given delete returns 409 for assigned role, UI shows conflict state and keeps row visible.
- AC-04: Given built-in Super Admin role is selected, permission controls are read-only.

Test Requirements: Unit form mapping tests; integration mocked CRUD responses; E2E/manual Super Admin role management walkthrough.

Observability Requirements: No runtime metric required.

Audit Requirements: UI must surface backend action result ids when present.

Migration / Rollback Requirements: No migration. Rollback hides role management route.

Documentation Requirements: Document role UI states.

Risks: Editable Super Admin UI can encourage unsafe role changes.

Definition of Done: Global DoD applies.

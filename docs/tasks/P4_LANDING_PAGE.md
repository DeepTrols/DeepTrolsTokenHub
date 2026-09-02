# P4 Landing Page Sprint Tasks

## Epic Code Audit

Existing:

- `ai-nuxt` is already Nuxt 4 + Vue 3 + TypeScript with Composition API and `<script setup>`.
- Tailwind CSS v4 Vite plugin and SCSS are present.
- Pricing/status/ratio Nitro endpoints currently return local static JSON.
- Backend already exposes public APIs for pricing, site, rankings, and stats.

Partial:

- Visual refactor preserved most static page structure, but data source remains split between static Nuxt endpoints and backend public APIs.
- Auth prototype routes exist in Nuxt, while real console authentication lives in the backend console.
- Visual parity was manually checked during refactor, but no repeatable screenshot baseline exists.

Missing:

- Runtime-configured backend API proxy.
- Async page data loading from public backend APIs.
- Fallback snapshot metadata for stale public data.
- Visual parity regression harness that protects the original design.
- Auth redirect boundary and retirement of Nuxt-only prototype auth routes.

Risks:

- Replacing static data with live APIs can accidentally change card order, modal content, spacing, or pricing display.
- Runtime proxy errors can make the public page blank.
- Prototype auth routes can confuse users if they look like the real console.

## Split Rule

Every Landing Page task is sized at 0.5d-2d and can be independently developed, reviewed, tested, accepted, and rolled back. Visual changes are out of scope unless explicitly listed.

### TH-P4-01

Task ID: TH-P4-01
Title: Nuxt Runtime Config
Phase: P4
Epic: Landing Page
Type: Frontend / Config
Priority: P2
Dependencies: TH-P05-09
Estimate: 0.5d

Objective: Add runtime configuration for the backend public API base URL.

Current State: Nuxt API endpoints use local static data and do not read backend base URL from runtime config.

Scope: Define public/private runtime config keys, local defaults, and environment variable names.

Out of Scope: Proxy implementation and page data mapping.

Implementation Notes: Use Nuxt runtime config, TypeScript types, SCSS/Tailwind v4 conventions, and existing project structure.

Acceptance Criteria:

- AC-01: Given no env var is set, local development uses documented default backend URL.
- AC-02: Given env var is set to `https://example.invalid`, runtime config exposes that value to server-side code.
- AC-03: Given frontend bundle is inspected, private server-only config is not emitted to client payload.

Test Requirements: Unit config helper tests; integration Nitro runtime config check; regression Nuxt build.

Observability Requirements: Log runtime config validation failure without secrets.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes runtime config keys.

Documentation Requirements: Update landing env configuration docs.

Risks: Leaking server-only config into client bundle can expose deployment internals.

Definition of Done: Global DoD applies.

### TH-P4-02

Task ID: TH-P4-02
Title: Nitro Public Proxy Base Client
Phase: P4
Epic: Landing Page
Type: Frontend / Nitro
Priority: P2
Dependencies: TH-P4-01
Estimate: 1d

Objective: Create a typed Nitro server client for backend public APIs.

Current State: Local Nuxt endpoints return static JSON instead of calling backend public APIs.

Scope: Add shared fetch client, base URL handling, status code mapping, and typed response wrapper.

Out of Scope: Page-specific mappers and caching.

Implementation Notes: Keep client server-side and avoid exposing backend internals to Vue components.

Acceptance Criteria:

- AC-01: Given backend returns 200 JSON, proxy client returns typed data to Nitro endpoint.
- AC-02: Given backend returns 500, proxy client maps it to a controlled public error payload.
- AC-03: Given backend request times out, proxy client returns timeout error state instead of hanging request.

Test Requirements: Unit client mapper tests; integration mocked backend responses; failure injection for timeout and 500.

Observability Requirements: Emit proxy request result metric or structured log by endpoint and status.

Audit Requirements: No audit event for public read-only proxy.

Migration / Rollback Requirements: No migration. Rollback restores static endpoint code.

Documentation Requirements: Document proxy client behavior.

Risks: Unbounded proxy waits can degrade first page render.

Definition of Done: Global DoD applies.

### TH-P4-03

Task ID: TH-P4-03
Title: Proxy Cache And Timeout
Phase: P4
Epic: Landing Page
Type: Frontend / Reliability
Priority: P2
Dependencies: TH-P4-02
Estimate: 1d

Objective: Add timeout and cache policy for Nuxt public API proxy calls.

Current State: Static local data does not require cache policy.

Scope: Configure request timeout, stale fallback decision, cache TTL, and cache invalidation behavior.

Out of Scope: Snapshot mapping and page rendering.

Implementation Notes: Use Nitro server capabilities and keep cache keys endpoint-specific.

Acceptance Criteria:

- AC-01: Given backend responds within timeout, proxy returns live data and cache is refreshed.
- AC-02: Given backend exceeds timeout and cache has fresh data, proxy returns cached data with `source=cache`.
- AC-03: Given backend exceeds timeout and no cache exists, proxy returns controlled error state.

Test Requirements: Unit cache key tests; integration timeout/cache behavior; regression build.

Observability Requirements: Emit cache hit, miss, stale, and timeout metrics.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback disables cache wrapper.

Documentation Requirements: Document TTL and timeout defaults.

Risks: Serving indefinitely stale model prices can mislead users.

Definition of Done: Global DoD applies.

### TH-P4-04

Task ID: TH-P4-04
Title: Proxy Fallback Snapshot Metadata
Phase: P4
Epic: Landing Page
Type: Frontend / Data
Priority: P2
Dependencies: TH-P4-03
Estimate: 1d

Objective: Attach clear metadata when public pages use fallback or cached data.

Current State: Static Nuxt data has no live/cache/stale metadata.

Scope: Add `source`, `generated_at`, `stale_at`, and optional `error_code` metadata to public endpoint responses.

Out of Scope: Visual redesign and backend API changes.

Implementation Notes: Metadata is for diagnostics and may be hidden in UI unless product requires display.

Acceptance Criteria:

- AC-01: Given live backend data is returned, metadata contains `source=live` and `generated_at`.
- AC-02: Given cached data is returned after backend timeout, metadata contains `source=cache` and previous `generated_at`.
- AC-03: Given no data can be returned, endpoint returns an error payload with `error_code` and no partial model list.

Test Requirements: Unit metadata mapper tests; integration live/cache/error cases; regression for existing endpoint consumers.

Observability Requirements: Structured logs include metadata source and endpoint.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback removes metadata wrapper.

Documentation Requirements: Document public endpoint metadata.

Risks: Hidden stale data can cause pricing trust issues.

Definition of Done: Global DoD applies.

### TH-P4-05

Task ID: TH-P4-05
Title: Pricing Payload Mapper
Phase: P4
Epic: Landing Page
Type: Frontend / Data Mapping
Priority: P2
Dependencies: TH-P4-04
Estimate: 1d

Objective: Map backend public pricing payload into the existing Nuxt model card view model.

Current State: Pricing page uses local model data matching the current visual layout.

Scope: Preserve card ordering, provider badges, pricing fields, descriptions, modal fields, and icon mapping inputs.

Out of Scope: Template styling changes and modal redesign.

Implementation Notes: Keep output view model TypeScript-typed and stable for Vue components.

Acceptance Criteria:

- AC-01: Given backend returns the same model ids as static data, mapper produces identical model card count and order.
- AC-02: Given backend omits optional description, mapper returns an empty-safe value and card layout does not throw.
- AC-03: Given unknown provider is returned, mapper uses documented fallback icon key without changing known provider icons.

Test Requirements: Unit mapper snapshots; integration mocked pricing endpoint; regression visual screenshot after TH-P4-10.

Observability Requirements: Emit mapper warning log for unknown provider without model secrets.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback points pricing page to static data.

Documentation Requirements: Document mapper fields and fallback rules.

Risks: Mapper drift can change visual ordering while code still compiles.

Definition of Done: Global DoD applies.

### TH-P4-06

Task ID: TH-P4-06
Title: Pricing Page Async Data
Phase: P4
Epic: Landing Page
Type: Frontend / Vue
Priority: P2
Dependencies: TH-P4-05
Estimate: 1d

Objective: Replace pricing page static loading with Nuxt async data while preserving the original visual output.

Current State: Pricing page renders static local data.

Scope: Use typed async data composable, loading state, error state, and mapped pricing cards.

Out of Scope: Styling changes, card redesign, and provider adapter work.

Implementation Notes: Use TypeScript, Composition API, `<script setup>`, existing components, Tailwind v4 utilities, and SCSS tokens.

Acceptance Criteria:

- AC-01: Given proxy returns model data, pricing page renders the same card grid count and labels as mapped payload.
- AC-02: Given proxy returns error, page renders controlled empty/error state without changing header and layout shell.
- AC-03: Given page is server-rendered, initial HTML contains pricing content when proxy succeeds.

Test Requirements: Unit composable tests; integration Nuxt page render tests; regression screenshot after TH-P4-10.

Observability Requirements: Log page data load failure with endpoint and request id.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback restores static data composable.

Documentation Requirements: Document data loading source.

Risks: Loading state that shifts layout can break the preserved visual design.

Definition of Done: Global DoD applies.

### TH-P4-07

Task ID: TH-P4-07
Title: Pricing Modal Live Data Regression
Phase: P4
Epic: Landing Page
Type: Frontend / Vue
Priority: P2
Dependencies: TH-P4-06
Estimate: 1d

Objective: Restore and verify the model detail modal against live pricing data.

Current State: Previous refactor temporarily lost or changed model card modal behavior.

Scope: Ensure card click opens detail modal with icon, provider, prices, context, features, and close behavior.

Out of Scope: Modal visual redesign and new fields not present in backend payload.

Implementation Notes: Use existing modal component if available; otherwise keep component responsibility focused.

Acceptance Criteria:

- AC-01: Given a model card is clicked, detail modal opens without navigating away.
- AC-02: Given modal opens for a known provider, provider icon matches the known provider mapping.
- AC-03: Given Escape key or close button is used, modal closes and page scroll position remains.
- AC-04: Given backend data changes price value, modal displays the same price as the card view model.

Test Requirements: Unit modal state tests; integration click/close tests; manual visual comparison against production.

Observability Requirements: No new runtime metric required.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback restores static modal data.

Documentation Requirements: Document modal data fields.

Risks: Modal mismatch can make model card pricing untrustworthy.

Definition of Done: Global DoD applies.

### TH-P4-08

Task ID: TH-P4-08
Title: Home Site Stats Mapper
Phase: P4
Epic: Landing Page
Type: Frontend / Data Mapping
Priority: P2
Dependencies: TH-P4-04
Estimate: 1d

Objective: Map backend public site and stats payloads into existing home page view models.

Current State: Home page uses local static content and numbers.

Scope: Map hero stats, model counts, channel counts, site copy fields, and fallback values.

Out of Scope: Hero layout redesign and backend changes.

Implementation Notes: Preserve original typography, spacing, and section order.

Acceptance Criteria:

- AC-01: Given backend returns site stats, mapper outputs all numeric home stats used by current components.
- AC-02: Given backend omits optional site copy, mapper uses existing static fallback text.
- AC-03: Given backend returns invalid numeric value, mapper rejects it and returns controlled error state.

Test Requirements: Unit mapper tests; integration mocked stats endpoint; regression home render.

Observability Requirements: Emit mapper warning for invalid numeric data.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback restores static home data.

Documentation Requirements: Document mapped fields.

Risks: Invalid stats can render misleading public claims.

Definition of Done: Global DoD applies.

### TH-P4-09

Task ID: TH-P4-09
Title: Home Rankings Featured Data
Phase: P4
Epic: Landing Page
Type: Frontend / Vue
Priority: P2
Dependencies: TH-P4-08
Estimate: 1d

Objective: Load home page rankings and featured model data from backend public APIs.

Current State: Home rankings are static or locally derived.

Scope: Use async data for rankings, featured models, loading state, and fallback state.

Out of Scope: Pricing page and ranking algorithm changes.

Implementation Notes: Preserve card dimensions and section spacing.

Acceptance Criteria:

- AC-01: Given backend returns ranking rows, home page renders rows in backend-provided order.
- AC-02: Given rankings endpoint returns empty list, page renders existing empty-safe state without layout collapse.
- AC-03: Given endpoint returns error, page renders fallback state and does not block hero render.

Test Requirements: Unit data mapper tests; integration Nuxt home render; manual visual comparison.

Observability Requirements: Log ranking load failures by endpoint.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback restores static ranking data.

Documentation Requirements: Document ranking data source.

Risks: Empty ranking layout can shift first viewport unexpectedly.

Definition of Done: Global DoD applies.

### TH-P4-10

Task ID: TH-P4-10
Title: Visual Parity Screenshot Baseline
Phase: P4
Epic: Landing Page
Type: Test / Visual Regression
Priority: P2
Dependencies: TH-P4-06, TH-P4-09
Estimate: 1.5d

Objective: Create repeatable screenshot checks that protect the original landing and pricing visual design.

Current State: Visual parity is checked manually and regressions have already appeared.

Scope: Capture desktop and mobile baselines for home, pricing, pricing modal, and header navigation states.

Out of Scope: Redesign and copy changes.

Implementation Notes: Compare against current approved local visual state or captured production reference after owner approval.

Acceptance Criteria:

- AC-01: Given visual test runs at desktop width, home and pricing screenshots match approved baseline within configured threshold.
- AC-02: Given visual test runs at mobile width, header, card grid, and modal do not overlap or overflow.
- AC-03: Given nav switches between home and pricing, header logo/nav positions remain fixed within threshold.
- AC-04: Given a pricing card opens modal, modal screenshot includes correct provider icon and detail content.

Test Requirements: Visual regression required; E2E/manual screenshot review; regression run in CI if project harness permits.

Observability Requirements: Test artifacts include route, viewport, and commit id.

Audit Requirements: No runtime audit event required.

Migration / Rollback Requirements: No migration. Rollback removes visual test artifacts only.

Documentation Requirements: Document baseline update process.

Risks: Incorrect baseline can freeze a broken visual state.

Definition of Done: Global DoD applies.

### TH-P4-11

Task ID: TH-P4-11
Title: Auth Redirect URL Builder
Phase: P4
Epic: Landing Page
Type: Frontend / Routing
Priority: P2
Dependencies: TH-P4-01
Estimate: 1d

Objective: Centralize links from Nuxt public site to the real backend console login and register flows.

Current State: Nuxt contains prototype auth routes while backend console is the real authenticated app.

Scope: Build typed helper for login, register, console, and return URL generation.

Out of Scope: Removing prototype pages and backend auth changes.

Implementation Notes: Avoid hardcoded local URLs in components.

Acceptance Criteria:

- AC-01: Given console base URL env var is set, login CTA points to that base URL plus login path.
- AC-02: Given return URL is present, generated redirect URL encodes it exactly once.
- AC-03: Given env var is missing, local dev URL matches documented default.

Test Requirements: Unit URL builder tests; integration component link render tests; regression header CTA tests.

Observability Requirements: No runtime metric required.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback restores existing link constants.

Documentation Requirements: Document auth redirect env variables.

Risks: Bad redirect encoding can break login or create open redirect risk.

Definition of Done: Global DoD applies.

### TH-P4-12

Task ID: TH-P4-12
Title: Nuxt Auth Prototype Route Retirement
Phase: P4
Epic: Landing Page
Type: Frontend / Routing
Priority: P2
Dependencies: TH-P4-11
Estimate: 1d

Objective: Remove or redirect Nuxt-only auth prototype routes to the real console boundary.

Current State: Nuxt prototype auth routes can look like the actual product login.

Scope: Redirect prototype login/register/console routes to configured real console URLs or remove them from routing.

Out of Scope: Backend console implementation.

Implementation Notes: Keep public landing routes intact.

Acceptance Criteria:

- AC-01: Given user visits Nuxt `/login`, response redirects to configured real console login.
- AC-02: Given user visits Nuxt `/console`, response redirects to configured real console entry.
- AC-03: Given user visits home or pricing, public pages still render normally.

Test Requirements: Integration route redirect tests; regression home/pricing route tests; manual browser check.

Observability Requirements: Log redirect route and target host without tokens.

Audit Requirements: No audit event required.

Migration / Rollback Requirements: No migration. Rollback restores prototype route files.

Documentation Requirements: Document retired routes and redirect behavior.

Risks: Leaving prototype auth visible can confuse user testing and production support.

Definition of Done: Global DoD applies.

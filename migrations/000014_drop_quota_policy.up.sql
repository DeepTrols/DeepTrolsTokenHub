-- Remove the quota & route-policy feature (2026-08-25):
--   - quota_pools / quota_allocations / quota_ledger
--   - route_policies (and usage_logs.route_policy_id FK)
--   - tenant_models.quota_enabled
ALTER TABLE usage_logs DROP COLUMN IF EXISTS route_policy_id;
ALTER TABLE tenant_models DROP COLUMN IF EXISTS quota_enabled;
DROP TABLE IF EXISTS route_policies;
DROP TABLE IF EXISTS quota_ledger;
DROP TABLE IF EXISTS quota_allocations;
DROP TABLE IF EXISTS quota_pools;

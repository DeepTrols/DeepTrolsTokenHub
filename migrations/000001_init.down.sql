-- 000001_init.down.sql
-- Rollback core schema

DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS reconciliation_diffs;
DROP TABLE IF EXISTS reconciliation_runs;
DROP TABLE IF EXISTS provider_evidence;
DROP TABLE IF EXISTS charge_lines;
DROP TABLE IF EXISTS usage_logs;
DROP TABLE IF EXISTS route_policies;
DROP TABLE IF EXISTS channel_instances;
DROP TABLE IF EXISTS channels;
DROP TABLE IF EXISTS quota_ledger;
DROP TABLE IF EXISTS quota_allocations;
DROP TABLE IF EXISTS quota_pools;
DROP TABLE IF EXISTS wallet_transactions;
DROP TABLE IF EXISTS wallets;
DROP TABLE IF EXISTS tenant_models;
DROP TABLE IF EXISTS model_pricing;
DROP TABLE IF EXISTS models;
DROP TABLE IF EXISTS tenant_domains;
DROP TABLE IF EXISTS tenants;
DROP TABLE IF EXISTS api_key_spend;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS login_history;
DROP TABLE IF EXISTS users;

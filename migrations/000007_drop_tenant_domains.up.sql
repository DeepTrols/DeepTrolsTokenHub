-- 000007_drop_tenant_domains.up.sql
-- Remove the tenant_domains table. There are no per-tenant custom domains —
-- every tenant is served on the platform's own domain (localhost), so the
-- tenant is resolved from the account's active membership, not the Host header.

DROP TABLE IF EXISTS tenant_domains;

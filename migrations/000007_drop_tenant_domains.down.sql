-- 000007_drop_tenant_domains.down.sql
-- Recreate tenant_domains (used by the old Host-header-based tenant middleware).

CREATE TABLE tenant_domains (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    domain VARCHAR(255) NOT NULL UNIQUE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenant_domains_domain ON tenant_domains(domain);

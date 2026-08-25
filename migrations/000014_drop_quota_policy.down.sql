-- Restore the quota & route-policy schema (rollback of the removal).
CREATE TABLE quota_pools (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    model_id UUID REFERENCES models(id),
    dimension VARCHAR(64) NOT NULL,
    total_amount BIGINT NOT NULL DEFAULT 0,
    allocated_amount BIGINT NOT NULL DEFAULT 0,
    used_amount BIGINT NOT NULL DEFAULT 0,
    unit_name VARCHAR(64) NOT NULL DEFAULT 'token',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE quota_allocations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pool_id UUID NOT NULL REFERENCES quota_pools(id),
    user_id UUID NOT NULL REFERENCES users(id),
    allocated_amount BIGINT NOT NULL,
    used_amount BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT quota_allocations_pool_user_unique UNIQUE (pool_id, user_id)
);

CREATE TABLE quota_ledger (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    allocation_id UUID NOT NULL REFERENCES quota_allocations(id),
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    action VARCHAR(32) NOT NULL
        CHECK (action IN ('grant', 'allocate', 'reclaim', 'consume', 'restore')),
    amount BIGINT NOT NULL,
    balance_after BIGINT NOT NULL,
    reference_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE route_policies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    tenant_id UUID,
    user_level VARCHAR(64),
    model_id UUID REFERENCES models(id),
    priority INT NOT NULL DEFAULT 0,
    candidate_channel_ids UUID[] NOT NULL,
    fallback_policy VARCHAR(32) NOT NULL DEFAULT 'disabled'
        CHECK (fallback_policy IN ('disabled', 'tenant_default', 'shared_allowed', 'next_policy')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE usage_logs ADD COLUMN route_policy_id UUID REFERENCES route_policies(id);
ALTER TABLE tenant_models ADD COLUMN quota_enabled BOOLEAN NOT NULL DEFAULT FALSE;

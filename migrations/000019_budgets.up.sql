-- ============================================================================
-- Tenant budgets and increase-request approvals (Phase 1 governance).
-- ============================================================================

CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    period VARCHAR(16) NOT NULL DEFAULT 'monthly'
        CHECK (period IN ('monthly', 'total')),
    limit_amount DECIMAL(18,6) NOT NULL DEFAULT 0,
    spent_amount DECIMAL(18,6) NOT NULL DEFAULT 0,
    status VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, period)
);

CREATE TABLE budget_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    requested_amount DECIMAL(18,6) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected')),
    reviewer_id UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_budget_requests_status ON budget_requests(status, created_at);

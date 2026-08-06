-- 000001_init.up.sql
-- Core schema for AI Token Aggregation Platform MVP

-- Extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- Users & Authentication
-- ============================================================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'banned', 'deleted')),
    totp_secret VARCHAR(255),
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE login_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    ip_address VARCHAR(45),
    user_agent TEXT,
    success BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_login_history_user ON login_history(user_id, created_at DESC);

-- ============================================================================
-- API Keys (6-boundary governance)
-- ============================================================================

CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    tenant_id UUID,
    key_prefix VARCHAR(12) NOT NULL,
    key_hash VARCHAR(255) NOT NULL UNIQUE,
    masked_key VARCHAR(64) NOT NULL,
    name VARCHAR(255),
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled', 'revoked', 'over_limit')),
    allowed_models TEXT[],
    source_whitelist TEXT[],
    cumulative_limit DECIMAL(18,6),
    weekly_limit DECIMAL(18,6),
    monthly_limit DECIMAL(18,6),
    over_limit_action VARCHAR(32) NOT NULL DEFAULT 'block'
        CHECK (over_limit_action IN ('block', 'warn')),
    last_used_at TIMESTAMPTZ,
    last_7d_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_api_keys_user ON api_keys(user_id);
CREATE INDEX idx_api_keys_tenant ON api_keys(tenant_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_status ON api_keys(status);

CREATE TABLE api_key_spend (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    api_key_id UUID NOT NULL REFERENCES api_keys(id),
    period_type VARCHAR(16) NOT NULL CHECK (period_type IN ('cumulative', 'weekly', 'monthly')),
    period_start TIMESTAMPTZ,
    period_end TIMESTAMPTZ,
    total_cost DECIMAL(18,6) NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (api_key_id, period_type, period_start)
);

-- ============================================================================
-- Tenants / OEM
-- ============================================================================

CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_review'
        CHECK (status IN ('pending_review', 'active', 'suspended', 'terminated', 'rejected')),
    owner_id UUID REFERENCES users(id),
    brand_config JSONB NOT NULL DEFAULT '{}',
    runtime_config JSONB NOT NULL DEFAULT '{}',
    settlement_config JSONB NOT NULL DEFAULT '{}',
    status_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tenant_domains (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    domain VARCHAR(255) NOT NULL UNIQUE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tenant_domains_domain ON tenant_domains(domain);

-- ============================================================================
-- Models & Pricing
-- ============================================================================

CREATE TABLE models (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code VARCHAR(128) NOT NULL UNIQUE,
    provider VARCHAR(64) NOT NULL,
    category VARCHAR(64) NOT NULL,
    display_name VARCHAR(255),
    description TEXT,
    context_window INT,
    max_output_tokens INT,
    capabilities JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'beta', 'deprecated', 'inactive')),
    release_stage VARCHAR(32) NOT NULL DEFAULT 'beta'
        CHECK (release_stage IN ('GA', 'beta', 'unsupported')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE model_pricing (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    model_id UUID NOT NULL REFERENCES models(id),
    tenant_id UUID,
    request_type VARCHAR(64) NOT NULL,
    pricing_dimension VARCHAR(64) NOT NULL,
    unit_name VARCHAR(64) NOT NULL,
    unit_price DECIMAL(18,10) NOT NULL,
    currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    upstream_cost DECIMAL(18,10),
    conditions JSONB,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_model_pricing_unique
    ON model_pricing(model_id, tenant_id, request_type, pricing_dimension)
    WHERE tenant_id IS NOT NULL;

CREATE TABLE tenant_models (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    model_id UUID NOT NULL REFERENCES models(id),
    is_listed BOOLEAN NOT NULL DEFAULT FALSE,
    allow_payg BOOLEAN NOT NULL DEFAULT FALSE,
    quota_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, model_id)
);

-- ============================================================================
-- Wallets & Transactions
-- ============================================================================

CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id),
    tenant_id UUID,
    balance DECIMAL(18,6) NOT NULL DEFAULT 0,
    frozen DECIMAL(18,6) NOT NULL DEFAULT 0,
    currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    version BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, tenant_id)
);

CREATE TABLE wallet_transactions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    wallet_id UUID NOT NULL REFERENCES wallets(id),
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    tx_type VARCHAR(32) NOT NULL
        CHECK (tx_type IN ('topup', 'charge', 'refund', 'reserve', 'release', 'transfer_in', 'transfer_out', 'compensate')),
    amount DECIMAL(18,6) NOT NULL,
    balance_before DECIMAL(18,6) NOT NULL,
    balance_after DECIMAL(18,6) NOT NULL,
    reference_type VARCHAR(64),
    reference_id UUID,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_wallet_tx_wallet ON wallet_transactions(wallet_id, created_at DESC);

-- ============================================================================
-- Quota Pools
-- ============================================================================

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
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

-- ============================================================================
-- Channels & Routing
-- ============================================================================

CREATE TABLE channels (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    model_id UUID NOT NULL REFERENCES models(id),
    tenant_id UUID,
    pool_type VARCHAR(32) NOT NULL DEFAULT 'shared'
        CHECK (pool_type IN ('shared', 'dedicated', 'mixed')),
    health_score INT NOT NULL DEFAULT 100,
    health_status VARCHAR(32) NOT NULL DEFAULT 'healthy'
        CHECK (health_status IN ('healthy', 'degraded', 'unhealthy')),
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive', 'pending_setup', 'disabled')),
    weight INT NOT NULL DEFAULT 100,
    max_concurrency INT NOT NULL DEFAULT 10,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE channel_instances (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    channel_id UUID NOT NULL REFERENCES channels(id),
    instance_type VARCHAR(64) NOT NULL,
    base_url VARCHAR(512) NOT NULL,
    provider_route VARCHAR(255),
    current_load INT NOT NULL DEFAULT 0,
    max_load INT NOT NULL DEFAULT 10,
    config JSONB NOT NULL DEFAULT '{}',
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'inactive', 'pending')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

-- ============================================================================
-- Usage Logs & Charge Lines
-- ============================================================================

CREATE TABLE usage_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id UUID,
    user_id UUID NOT NULL REFERENCES users(id),
    api_key_id UUID NOT NULL REFERENCES api_keys(id),
    request_id VARCHAR(255) NOT NULL,
    request_type VARCHAR(64) NOT NULL,
    public_model_code VARCHAR(128) NOT NULL,
    upstream_model_code VARCHAR(255),
    channel_id UUID REFERENCES channels(id),
    instance_id UUID REFERENCES channel_instances(id),
    route_policy_id UUID REFERENCES route_policies(id),
    provider_request_id VARCHAR(255),
    usage_source VARCHAR(32) NOT NULL
        CHECK (usage_source IN ('upstream', 'final_chunk', 'estimated')),
    usage_raw JSONB,
    usage_normalized JSONB,
    estimated_cost DECIMAL(18,6),
    list_cost DECIMAL(18,6) NOT NULL,
    discount_amount DECIMAL(18,6) NOT NULL DEFAULT 0,
    final_cost DECIMAL(18,6) NOT NULL,
    upstream_cost DECIMAL(18,6),
    currency VARCHAR(8) NOT NULL DEFAULT 'CNY',
    price_snapshot JSONB,
    quota_deducted BIGINT NOT NULL DEFAULT 0,
    wallet_charged DECIMAL(18,6) NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'completed'
        CHECK (status IN ('completed', 'failed', 'partial', 'refunded')),
    error_code VARCHAR(64),
    error_message TEXT,
    request_summary TEXT,
    response_summary TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_usage_logs_user ON usage_logs(user_id, created_at DESC);
CREATE INDEX idx_usage_logs_key ON usage_logs(api_key_id, created_at DESC);
CREATE INDEX idx_usage_logs_tenant ON usage_logs(tenant_id, created_at DESC);
CREATE INDEX idx_usage_logs_request ON usage_logs(request_id);
CREATE INDEX idx_usage_logs_model ON usage_logs(public_model_code, created_at DESC);
CREATE INDEX idx_usage_logs_created ON usage_logs(created_at DESC);

CREATE TABLE charge_lines (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    usage_log_id UUID NOT NULL REFERENCES usage_logs(id),
    dimension VARCHAR(64) NOT NULL,
    unit_name VARCHAR(64) NOT NULL,
    quantity BIGINT NOT NULL,
    unit_price DECIMAL(18,10) NOT NULL,
    line_cost DECIMAL(18,6) NOT NULL,
    discount_applied DECIMAL(18,6) NOT NULL DEFAULT 0,
    price_source VARCHAR(64),
    price_version INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_charge_lines_log ON charge_lines(usage_log_id);

-- ============================================================================
-- Provider Evidence (L1)
-- ============================================================================

CREATE TABLE provider_evidence (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    usage_log_id UUID REFERENCES usage_logs(id),
    provider VARCHAR(64) NOT NULL,
    provider_request_id VARCHAR(255),
    request_body JSONB,
    response_body JSONB,
    status_code INT,
    duration_ms INT,
    usage_raw JSONB,
    provider_cost DECIMAL(18,6),
    provider_currency VARCHAR(8),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_provider_evidence_log ON provider_evidence(usage_log_id);
CREATE INDEX idx_provider_evidence_provider ON provider_evidence(provider, created_at DESC);

-- ============================================================================
-- Reconciliation
-- ============================================================================

CREATE TABLE reconciliation_runs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    level VARCHAR(8) NOT NULL CHECK (level IN ('L0', 'L1', 'L2', 'L3')),
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    total_requests INT,
    diff_count INT,
    status VARCHAR(32) NOT NULL DEFAULT 'running'
        CHECK (status IN ('running', 'completed', 'failed')),
    report JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE reconciliation_diffs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    run_id UUID NOT NULL REFERENCES reconciliation_runs(id),
    usage_log_id UUID REFERENCES usage_logs(id),
    diff_type VARCHAR(64) NOT NULL,
    diff_detail JSONB,
    severity VARCHAR(16) NOT NULL DEFAULT 'info'
        CHECK (severity IN ('critical', 'warning', 'info')),
    resolution_status VARCHAR(32) NOT NULL DEFAULT 'open'
        CHECK (resolution_status IN ('open', 'reviewing', 'resolved', 'ignored')),
    resolution_note TEXT,
    reviewer_id UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

-- ============================================================================
-- Outbox (Durable async billing)
-- ============================================================================

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_type VARCHAR(64) NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL UNIQUE,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX idx_outbox_status ON outbox_events(status, created_at);

-- ============================================================================
-- Audit Log
-- ============================================================================

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_id UUID REFERENCES users(id),
    actor_type VARCHAR(32) NOT NULL,
    tenant_id UUID,
    action VARCHAR(128) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    resource_id UUID,
    old_value JSONB,
    new_value JSONB,
    reason TEXT,
    ip_address VARCHAR(45),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_actor ON audit_logs(actor_id, created_at DESC);
CREATE INDEX idx_audit_logs_tenant ON audit_logs(tenant_id, created_at DESC);
CREATE INDEX idx_audit_logs_resource ON audit_logs(resource_type, resource_id);

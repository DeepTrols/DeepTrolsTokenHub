-- ============================================================================
-- Billing synchronization (ported from TokenHub billing module, Apache-2.0)
-- 上游账单同步：OneAPI / NewAPI / 阿里云 → billing_records，供对账 L3 使用。
-- ============================================================================

CREATE TABLE billing_connectors (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(32) NOT NULL,
    base_url VARCHAR(512) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    schedule_interval_minutes INT NOT NULL DEFAULT 0,
    config JSONB NOT NULL DEFAULT '{}',
    credential_ciphertext TEXT NOT NULL DEFAULT '',
    checkpoint TEXT NOT NULL DEFAULT '',
    last_synced_through TIMESTAMPTZ,
    last_sync_status VARCHAR(32),
    last_sync_message TEXT,
    last_sync_at TIMESTAMPTZ,
    next_sync_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_billing_connectors_status ON billing_connectors(status, next_sync_at);

CREATE TABLE billing_sync_runs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    connector_id UUID NOT NULL REFERENCES billing_connectors(id),
    trigger VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    range_start TIMESTAMPTZ NOT NULL,
    range_end TIMESTAMPTZ NOT NULL,
    cursor_start TEXT NOT NULL DEFAULT '',
    cursor_end TEXT NOT NULL DEFAULT '',
    pages_fetched INT NOT NULL DEFAULT 0,
    attempts INT NOT NULL DEFAULT 0,
    records_seen INT NOT NULL DEFAULT 0,
    records_inserted INT NOT NULL DEFAULT 0,
    records_updated INT NOT NULL DEFAULT 0,
    error_code VARCHAR(64),
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ
);

CREATE INDEX idx_billing_sync_runs_connector ON billing_sync_runs(connector_id, started_at DESC);

CREATE TABLE billing_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    connector_id UUID NOT NULL REFERENCES billing_connectors(id),
    external_id VARCHAR(255) NOT NULL,
    source_type VARCHAR(64) NOT NULL DEFAULT '',
    account_id VARCHAR(255) NOT NULL DEFAULT '',
    service VARCHAR(128) NOT NULL DEFAULT '',
    product VARCHAR(128) NOT NULL DEFAULT '',
    model VARCHAR(128) NOT NULL DEFAULT '',
    currency VARCHAR(16) NOT NULL DEFAULT '',
    gross_amount TEXT NOT NULL DEFAULT '',
    discount_amount TEXT NOT NULL DEFAULT '',
    tax_amount TEXT NOT NULL DEFAULT '',
    refund_amount TEXT NOT NULL DEFAULT '',
    net_amount TEXT NOT NULL DEFAULT '',
    usage_quantity BIGINT NOT NULL DEFAULT 0,
    usage_unit VARCHAR(64) NOT NULL DEFAULT '',
    usage_start_at TIMESTAMPTZ NOT NULL,
    usage_end_at TIMESTAMPTZ NOT NULL,
    source_timezone VARCHAR(64) NOT NULL DEFAULT '',
    billing_period VARCHAR(64) NOT NULL DEFAULT '',
    external_request_id VARCHAR(255) NOT NULL DEFAULT '',
    raw_snapshot_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (connector_id, external_id)
);

CREATE INDEX idx_billing_records_usage_start ON billing_records(connector_id, usage_start_at);

CREATE TABLE billing_raw_snapshots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    connector_id UUID NOT NULL REFERENCES billing_connectors(id),
    external_id VARCHAR(255) NOT NULL,
    payload_hash VARCHAR(64) NOT NULL,
    payload TEXT NOT NULL DEFAULT '',
    captured_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (connector_id, external_id, payload_hash)
);

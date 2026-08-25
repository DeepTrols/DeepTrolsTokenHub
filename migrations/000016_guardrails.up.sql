-- ============================================================================
-- Guardrails content policy (ported from TokenHub, Apache-2.0)
-- 出站内容策略：策略 + 检测项 + 绑定（项目/租户维度）
-- ============================================================================

CREATE TABLE guardrail_policies (
    id TEXT PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    config_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE guardrail_detection_items (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES guardrail_policies(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    detector_type VARCHAR(64) NOT NULL,
    action VARCHAR(32) NOT NULL,
    config_version INT NOT NULL DEFAULT 1,
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_guardrail_items_policy ON guardrail_detection_items(policy_id);

CREATE TABLE guardrail_policy_bindings (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES guardrail_policies(id) ON DELETE CASCADE,
    scope_type VARCHAR(32) NOT NULL,
    scope_id VARCHAR(255) NOT NULL DEFAULT '',
    checkpoint VARCHAR(64) NOT NULL DEFAULT 'before_provider',
    protocol VARCHAR(32) NOT NULL DEFAULT 'all',
    config_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (policy_id, scope_type, scope_id, checkpoint, protocol)
);

CREATE INDEX idx_guardrail_bindings_scope ON guardrail_policy_bindings(scope_type, scope_id);

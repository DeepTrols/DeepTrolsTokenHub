-- 000025_redemption_codes.up.sql
CREATE TABLE IF NOT EXISTS redemption_codes (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code       VARCHAR(64) NOT NULL UNIQUE,
    amount     DECIMAL(18,6) NOT NULL,
    status     VARCHAR(16) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'used', 'expired')),
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    used_by    UUID REFERENCES users(id),
    used_at    TIMESTAMPTZ
);
CREATE INDEX idx_redemption_codes_status ON redemption_codes(status);

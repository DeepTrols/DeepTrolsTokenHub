-- ============================================================================
-- Per-API-key minute quota buckets (RPM / TPM) for gateway admission.
-- rate_limit_rpm / rate_limit_tpm = 0 means unlimited.
-- ============================================================================

ALTER TABLE api_keys
    ADD COLUMN rate_limit_rpm INT NOT NULL DEFAULT 0,
    ADD COLUMN rate_limit_tpm BIGINT NOT NULL DEFAULT 0;

CREATE TABLE api_key_quota_buckets (
    key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE CASCADE,
    bucket VARCHAR(14) NOT NULL, -- YYYYMMDDHHMM
    requests BIGINT NOT NULL DEFAULT 0,
    tokens BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (key_id, bucket)
);

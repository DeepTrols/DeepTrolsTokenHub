DROP TABLE IF EXISTS api_key_quota_buckets;

ALTER TABLE api_keys
    DROP COLUMN IF EXISTS rate_limit_rpm,
    DROP COLUMN IF EXISTS rate_limit_tpm;

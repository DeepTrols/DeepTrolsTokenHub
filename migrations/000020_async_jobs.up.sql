-- ============================================================================
-- Async generation jobs (Phase 4 video): the gateway records long-running
-- upstream tasks (e.g. 豆包 Seedance / 可灵) so clients can poll status,
-- download results and cancel. Wallet holds stay attached to the job until
-- completion or cancellation.
-- ============================================================================

CREATE TABLE async_jobs (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    api_key_id UUID,
    tenant_id UUID,
    model VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'processing'
        CHECK (status IN ('queued', 'processing', 'succeeded', 'failed', 'cancelled')),
    request_type VARCHAR(32) NOT NULL DEFAULT 'video',
    upstream_job_id VARCHAR(255),
    result_url TEXT,
    error TEXT,
    hold_tx_id UUID,
    request_id VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_async_jobs_user_created ON async_jobs (user_id, created_at DESC);
CREATE INDEX idx_async_jobs_status ON async_jobs (status);

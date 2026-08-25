-- ============================================================================
-- Gateway resource model (Phase 0): route strategy + sticky session +
-- instance cooldown / concurrency limit.
-- ============================================================================

ALTER TABLE channels
    ADD COLUMN strategy VARCHAR(16) NOT NULL DEFAULT 'priority_only'
        CHECK (strategy IN ('priority_only', 'cost', 'quality')),
    ADD COLUMN sticky_session BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN fallback_order INT NOT NULL DEFAULT 0;

ALTER TABLE channel_instances
    ADD COLUMN cooldown_until TIMESTAMPTZ,
    ADD COLUMN last_checked_at TIMESTAMPTZ,
    ADD COLUMN concurrency_limit INT NOT NULL DEFAULT 10;

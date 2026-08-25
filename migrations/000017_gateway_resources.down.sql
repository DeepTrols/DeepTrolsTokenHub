ALTER TABLE channel_instances
    DROP COLUMN IF EXISTS cooldown_until,
    DROP COLUMN IF EXISTS last_checked_at,
    DROP COLUMN IF EXISTS concurrency_limit;

ALTER TABLE channels
    DROP COLUMN IF EXISTS strategy,
    DROP COLUMN IF EXISTS sticky_session,
    DROP COLUMN IF EXISTS fallback_order;

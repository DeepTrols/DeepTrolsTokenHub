-- ============================================================================
-- quota_allocations: (pool_id, user_id) must be unique.
--
-- The admin quota allocation endpoint upserts via ON CONFLICT (pool_id,
-- user_id), which is invalid SQL without a matching unique constraint — the
-- endpoint has been returning 500 on every allocation. Team quota allocation
-- (enterprise admin → sub-account) relies on the same constraint for its
-- atomic upsert. There is no pre-existing allocation data (the broken upsert
-- never inserted rows), so the constraint can be added directly.
-- ============================================================================

ALTER TABLE quota_allocations
    ADD CONSTRAINT quota_allocations_pool_user_unique UNIQUE (pool_id, user_id);

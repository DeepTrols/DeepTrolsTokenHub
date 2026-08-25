-- No-op: the quota feature (incl. quota_allocations) was removed on
-- 2026-08-25. Fresh databases no longer create these tables and migration
-- 000014 drops them from existing databases; this file is kept as a no-op so
-- applied migration history stays valid.
SELECT 1;

-- 000010_backfill_last_used.up.sql
-- Backfill api_keys.last_used_at from usage_logs for keys that have usage
-- records but never had last_used_at written (the gateway did not record it
-- before 2026-08-19). Keys with recent usage also get last_7d_active = TRUE.

UPDATE api_keys k
SET last_used_at = u.last_used,
    last_7d_active = CASE
        WHEN u.last_used >= NOW() - INTERVAL '7 days' THEN TRUE
        ELSE k.last_7d_active
    END,
    updated_at = NOW()
FROM (
    SELECT api_key_id, MAX(created_at) AS last_used
    FROM usage_logs
    WHERE api_key_id IS NOT NULL
    GROUP BY api_key_id
) u
WHERE k.id = u.api_key_id
  AND k.last_used_at IS NULL;

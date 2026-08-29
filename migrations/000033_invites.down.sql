-- 000033_invites.down.sql
DROP INDEX IF EXISTS idx_users_invite_code;
ALTER TABLE users DROP COLUMN IF EXISTS invited_by;
ALTER TABLE users DROP COLUMN IF EXISTS invite_code;

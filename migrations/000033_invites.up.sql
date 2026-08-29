-- 000033_invites.up.sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS invite_code VARCHAR(32);
ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_by UUID REFERENCES users(id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_invite_code ON users(invite_code) WHERE invite_code IS NOT NULL;

-- Backfill existing users with deterministic invite codes.
UPDATE users
SET invite_code = 'DTP' || substr(replace(md5(id::text), '-', ''), 1, 8)
WHERE invite_code IS NULL OR invite_code = '';

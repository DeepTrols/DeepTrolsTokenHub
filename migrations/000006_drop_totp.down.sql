-- Restore TOTP/2FA columns on users (rollback of feature removal).
ALTER TABLE users ADD COLUMN totp_secret VARCHAR(255);
ALTER TABLE users ADD COLUMN totp_enabled BOOLEAN NOT NULL DEFAULT FALSE;

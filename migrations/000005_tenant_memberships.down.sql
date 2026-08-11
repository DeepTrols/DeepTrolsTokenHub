-- 000005_tenant_memberships.down.sql
-- Reverse the OEM multi-tenant migration.

DROP TABLE IF EXISTS tenant_invitations;
DROP TABLE IF EXISTS tenant_memberships;

ALTER TABLE tenants DROP COLUMN IF EXISTS business_license;
ALTER TABLE tenants DROP COLUMN IF EXISTS contact_phone;
ALTER TABLE tenants DROP COLUMN IF EXISTS contact_email;
ALTER TABLE tenants DROP COLUMN IF EXISTS credit_code;

ALTER TABLE users DROP COLUMN IF EXISTS avatar_url;
ALTER TABLE users DROP COLUMN IF EXISTS phone;
ALTER TABLE users DROP COLUMN IF EXISTS user_type;

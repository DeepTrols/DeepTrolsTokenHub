-- 000029_subscription_groups.up.sql
-- A subscription plan can grant access to a channel group; users may then
-- create API keys bound to that group (gateway FilterByGroup enforces routing).
ALTER TABLE subscription_plans ADD COLUMN IF NOT EXISTS group_name VARCHAR(64);

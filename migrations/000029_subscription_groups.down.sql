-- 000029_subscription_groups.down.sql
ALTER TABLE subscription_plans DROP COLUMN IF EXISTS group_name;

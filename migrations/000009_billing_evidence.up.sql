-- 计费证据链收尾（Step 1）：
-- 1. model_pricing 增加 price_version，定价每次变更递增，作为计费快照的版本证据
-- 2. usage_logs.usage_source 放行 cached（缓存命中证据落库，此前会被 CHECK 拒绝）

ALTER TABLE model_pricing ADD COLUMN price_version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE usage_logs DROP CONSTRAINT usage_logs_usage_source_check;
ALTER TABLE usage_logs ADD CONSTRAINT usage_logs_usage_source_check
  CHECK (usage_source IN ('upstream', 'final_chunk', 'estimated', 'cached'));

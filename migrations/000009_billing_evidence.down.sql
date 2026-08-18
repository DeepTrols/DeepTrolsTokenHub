ALTER TABLE model_pricing DROP COLUMN price_version;

ALTER TABLE usage_logs DROP CONSTRAINT usage_logs_usage_source_check;
ALTER TABLE usage_logs ADD CONSTRAINT usage_logs_usage_source_check
  CHECK (usage_source IN ('upstream', 'final_chunk', 'estimated'));

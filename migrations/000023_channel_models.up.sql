-- 000023_channel_models.up.sql
-- Channel ↔ model bindings that are discoverable/sync-able from upstream.
-- Routing prefers channel_models; the legacy channels.model_id is retained.

CREATE TABLE IF NOT EXISTS channel_models (
    channel_id     UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    model_id       UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
    upstream_model VARCHAR(255) NOT NULL,
    mapping        VARCHAR(255),
    enabled        BOOLEAN NOT NULL DEFAULT TRUE,
    discovered_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel_id, model_id)
);

CREATE INDEX idx_channel_models_upstream ON channel_models(channel_id, upstream_model);

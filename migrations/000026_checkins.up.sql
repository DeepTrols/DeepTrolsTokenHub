-- 000026_checkins.up.sql
CREATE TABLE IF NOT EXISTS checkins (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID NOT NULL REFERENCES users(id),
    checkin_date DATE NOT NULL,
    amount       DECIMAL(18,6) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, checkin_date)
);

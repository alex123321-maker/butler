CREATE TABLE IF NOT EXISTS channel_delivery_events (
    event_id          BIGSERIAL PRIMARY KEY,
    run_id            TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    session_key       TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    channel           TEXT NOT NULL,
    delivery_type     TEXT NOT NULL,
    state             TEXT NOT NULL,
    error_message     TEXT NOT NULL DEFAULT '',
    details_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_delivery_events_channel_check CHECK (channel IN ('telegram', 'web')),
    CONSTRAINT channel_delivery_events_state_check CHECK (state IN ('sent', 'waiting_reply', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_channel_delivery_events_run_id ON channel_delivery_events (run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_delivery_events_created_at ON channel_delivery_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_delivery_events_channel_state ON channel_delivery_events (channel, state, created_at DESC);

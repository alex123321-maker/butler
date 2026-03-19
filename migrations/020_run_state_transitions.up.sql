CREATE TABLE IF NOT EXISTS run_state_transitions (
    id              BIGSERIAL PRIMARY KEY,
    run_id          TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    from_state      TEXT NOT NULL,
    to_state        TEXT NOT NULL,
    triggered_by    TEXT NOT NULL DEFAULT '',
    metadata_json   JSONB NOT NULL DEFAULT '{}',
    transitioned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_run_state_transitions_run_id ON run_state_transitions (run_id, transitioned_at);

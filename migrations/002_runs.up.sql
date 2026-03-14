CREATE TABLE IF NOT EXISTS runs (
    run_id TEXT PRIMARY KEY,
    session_key TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    input_event_id TEXT NOT NULL,
    status TEXT NOT NULL,
    autonomy_mode TEXT NOT NULL,
    current_state TEXT NOT NULL,
    model_provider TEXT NOT NULL,
    provider_session_ref TEXT,
    lease_id TEXT,
    resumes_run_id TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    error_type TEXT,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_runs_session_started_at ON runs (session_key, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_current_state ON runs (current_state);

CREATE TABLE IF NOT EXISTS artifacts (
    artifact_id      TEXT PRIMARY KEY,
    run_id           TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    session_key      TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    artifact_type    TEXT NOT NULL,
    title            TEXT NOT NULL DEFAULT '',
    summary          TEXT NOT NULL DEFAULT '',
    content_text     TEXT NOT NULL DEFAULT '',
    content_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    content_format   TEXT NOT NULL DEFAULT 'text',
    source_type      TEXT NOT NULL DEFAULT '',
    source_ref       TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT artifacts_type_check CHECK (artifact_type IN ('assistant_final', 'doctor_report', 'tool_result', 'summary')),
    CONSTRAINT artifacts_format_check CHECK (content_format IN ('text', 'markdown', 'json'))
);

CREATE INDEX IF NOT EXISTS idx_artifacts_run_id ON artifacts (run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_artifacts_session_key ON artifacts (session_key, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_artifacts_type ON artifacts (artifact_type, created_at DESC);

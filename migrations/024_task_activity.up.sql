CREATE TABLE IF NOT EXISTS task_activity (
    activity_id      BIGSERIAL PRIMARY KEY,
    run_id           TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    session_key      TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    activity_type    TEXT NOT NULL,
    title            TEXT NOT NULL DEFAULT '',
    summary          TEXT NOT NULL DEFAULT '',
    details_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor_type       TEXT NOT NULL DEFAULT 'system',
    severity         TEXT NOT NULL DEFAULT 'info',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT task_activity_actor_type_check CHECK (actor_type IN ('system', 'agent', 'user', 'telegram_adapter', 'web_ui')),
    CONSTRAINT task_activity_severity_check CHECK (severity IN ('info', 'warning', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_task_activity_run_id ON task_activity (run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_activity_created_at ON task_activity (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_activity_severity ON task_activity (severity, created_at DESC);

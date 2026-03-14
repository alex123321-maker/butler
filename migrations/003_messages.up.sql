CREATE TABLE IF NOT EXISTS messages (
    message_id TEXT PRIMARY KEY,
    session_key TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    run_id TEXT REFERENCES runs (run_id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    tool_call_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_session_created_at ON messages (session_key, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_run_id ON messages (run_id);

CREATE TABLE IF NOT EXISTS memory_working (
    id BIGSERIAL PRIMARY KEY,
    session_key TEXT NOT NULL UNIQUE REFERENCES sessions (session_key) ON DELETE CASCADE,
    run_id TEXT REFERENCES runs (run_id) ON DELETE SET NULL,
    goal TEXT NOT NULL DEFAULT '',
    entities JSONB NOT NULL DEFAULT '{}'::jsonb,
    pending_steps JSONB NOT NULL DEFAULT '[]'::jsonb,
    scratch JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_working_run_id ON memory_working (run_id);

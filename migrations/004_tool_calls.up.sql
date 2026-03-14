CREATE TABLE IF NOT EXISTS tool_calls (
    tool_call_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    tool_name TEXT NOT NULL,
    args JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL,
    runtime_target TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    result JSONB,
    error JSONB
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_run_id ON tool_calls (run_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_status ON tool_calls (status);

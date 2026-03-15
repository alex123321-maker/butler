CREATE TABLE IF NOT EXISTS credential_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    run_id TEXT NOT NULL DEFAULT '',
    tool_call_id TEXT NOT NULL DEFAULT '',
    alias TEXT NOT NULL,
    field TEXT NOT NULL,
    tool_name TEXT NOT NULL,
    target_domain TEXT NOT NULL DEFAULT '',
    decision TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_credential_audit_logs_run_tool ON credential_audit_logs (run_id, tool_call_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_credential_audit_logs_alias_created_at ON credential_audit_logs (alias, created_at DESC);

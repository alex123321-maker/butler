CREATE TABLE IF NOT EXISTS approvals (
    approval_id        TEXT PRIMARY KEY,
    run_id             TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    session_key        TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    tool_call_id       TEXT NOT NULL,
    status             TEXT NOT NULL,
    requested_via      TEXT NOT NULL,
    resolved_via       TEXT,
    tool_name          TEXT NOT NULL,
    args_json          JSONB NOT NULL DEFAULT '{}'::jsonb,
    risk_level         TEXT NOT NULL DEFAULT 'medium',
    summary            TEXT NOT NULL DEFAULT '',
    details_json       JSONB NOT NULL DEFAULT '{}'::jsonb,
    requested_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at        TIMESTAMPTZ,
    resolved_by        TEXT NOT NULL DEFAULT '',
    resolution_reason  TEXT NOT NULL DEFAULT '',
    expires_at         TIMESTAMPTZ,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT approvals_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'failed')),
    CONSTRAINT approvals_requested_via_check CHECK (requested_via IN ('telegram', 'web', 'both')),
    CONSTRAINT approvals_resolved_via_check CHECK (resolved_via IS NULL OR resolved_via IN ('telegram', 'web', 'system')),
    CONSTRAINT approvals_risk_level_check CHECK (risk_level IN ('low', 'medium', 'high'))
);

CREATE INDEX IF NOT EXISTS idx_approvals_run_id ON approvals (run_id);
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals (status);
CREATE INDEX IF NOT EXISTS idx_approvals_requested_at ON approvals (requested_at DESC);
CREATE INDEX IF NOT EXISTS idx_approvals_session_key ON approvals (session_key, requested_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_approvals_tool_call_pending ON approvals (tool_call_id) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS approval_events (
    event_id           BIGSERIAL PRIMARY KEY,
    approval_id        TEXT NOT NULL REFERENCES approvals (approval_id) ON DELETE CASCADE,
    run_id             TEXT NOT NULL REFERENCES runs (run_id) ON DELETE CASCADE,
    session_key        TEXT NOT NULL REFERENCES sessions (session_key) ON DELETE CASCADE,
    event_type         TEXT NOT NULL,
    status_before      TEXT,
    status_after       TEXT,
    actor_type         TEXT NOT NULL DEFAULT 'system',
    actor_id           TEXT NOT NULL DEFAULT '',
    reason             TEXT NOT NULL DEFAULT '',
    metadata_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT approval_events_actor_type_check CHECK (actor_type IN ('system', 'telegram', 'web', 'operator'))
);

CREATE INDEX IF NOT EXISTS idx_approval_events_approval_id ON approval_events (approval_id, created_at);
CREATE INDEX IF NOT EXISTS idx_approval_events_run_id ON approval_events (run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_approval_events_created_at ON approval_events (created_at DESC);

CREATE TABLE session_ownership (
    session_key TEXT PRIMARY KEY REFERENCES sessions(session_key) ON DELETE CASCADE,
    lease_id TEXT NOT NULL DEFAULT '',
    run_id TEXT NOT NULL DEFAULT '',
    owner_id TEXT NOT NULL DEFAULT '',
    fence_epoch BIGINT NOT NULL DEFAULT 0 CHECK (fence_epoch >= 0),
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX session_ownership_lease_id ON session_ownership(lease_id) WHERE lease_id <> '';

CREATE TABLE accepted_inputs (
    input_id TEXT PRIMARY KEY,
    session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
    session_sequence BIGINT GENERATED ALWAYS AS IDENTITY,
    event_id TEXT NOT NULL,
    dedup_key TEXT NOT NULL,
    source TEXT NOT NULL,
    owner_id TEXT NOT NULL,
    event_json JSONB NOT NULL,
    task_id TEXT REFERENCES tasks(task_id),
    run_id TEXT REFERENCES runs(run_id),
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','waiting_for_user','blocked','completed','failed','expired','cancelled')),
    checkpoint TEXT NOT NULL DEFAULT 'accepted',
    checkpoint_json JSONB NOT NULL DEFAULT '{}',
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts > 0),
    claim_id TEXT NOT NULL DEFAULT '',
    claim_expires_at TIMESTAMPTZ,
    not_before TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() + INTERVAL '24 hours',
    accepted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    error_code TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    UNIQUE(session_key, source, dedup_key)
);
CREATE INDEX accepted_inputs_heads ON accepted_inputs(session_key, session_sequence) WHERE status NOT IN ('completed','failed','expired','cancelled');
CREATE INDEX accepted_inputs_ready ON accepted_inputs(not_before, accepted_at) WHERE status IN ('queued','running');

-- Existing model/tool attempts have no durable replay boundary. Preserve them,
-- but never reinterpret a legacy waiting process as permission to dispatch.
UPDATE tasks SET status='blocked', updated_at=clock_timestamp()
 WHERE task_id IN (SELECT task_id FROM runs WHERE current_state='awaiting_approval')
 AND status NOT IN ('completed','cancelled');

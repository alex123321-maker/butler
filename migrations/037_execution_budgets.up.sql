CREATE TABLE execution_budget_scopes (
    scope_id TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('task','mandate','owner_day','conversation')),
    owner_id TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    limits_json JSONB NOT NULL,
    used_json JSONB NOT NULL,
    deadline TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(kind, owner_id, subject_id)
);
CREATE TABLE execution_budget_reservations (
    reservation_id TEXT PRIMARY KEY,
    owner_id TEXT NOT NULL,
    operation_id TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    request_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(owner_id, operation_id)
);
CREATE INDEX execution_budget_reservations_owner_created_idx ON execution_budget_reservations(owner_id,created_at);

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

CREATE TABLE execution_model_attempts (
 operation_id TEXT PRIMARY KEY,
 run_id TEXT NOT NULL,
 reservation_id TEXT NOT NULL REFERENCES execution_budget_reservations(reservation_id),
 provider TEXT NOT NULL,model TEXT NOT NULL,
 input_allowance BIGINT NOT NULL CHECK(input_allowance>=0),
 output_allowance BIGINT NOT NULL CHECK(output_allowance>0),
 actual_input_tokens BIGINT CHECK(actual_input_tokens>=0),
 actual_output_tokens BIGINT CHECK(actual_output_tokens>=0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),observed_at TIMESTAMPTZ
);
CREATE INDEX execution_model_attempts_run_idx ON execution_model_attempts(run_id,created_at);

CREATE TABLE execution_loop_observations (
 effect_id TEXT PRIMARY KEY,
 owner_id TEXT NOT NULL, task_root TEXT NOT NULL,
 state_key TEXT NOT NULL, call_key TEXT NOT NULL,
 category TEXT NOT NULL,result_digest TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX execution_loop_observations_lookup ON execution_loop_observations(owner_id,task_root,state_key,call_key,category,result_digest);

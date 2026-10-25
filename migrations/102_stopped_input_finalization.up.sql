-- Attempt failure is separate from the unresolved outcome of an action.
CREATE TABLE input_stopped_attempts (
 input_id TEXT PRIMARY KEY REFERENCES accepted_inputs(input_id) ON DELETE CASCADE,
 run_id TEXT NOT NULL UNIQUE REFERENCES runs(run_id) ON DELETE CASCADE,
 owner_id TEXT NOT NULL,
 session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
 classification TEXT NOT NULL CHECK(classification IN ('known','uncertain')),
 reason TEXT NOT NULL CHECK(reason IN ('no_uncertainty','journal_uncertain','missing_receipt')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 checked_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX input_stopped_uncertain ON input_stopped_attempts(owner_id,session_key) WHERE classification='uncertain';

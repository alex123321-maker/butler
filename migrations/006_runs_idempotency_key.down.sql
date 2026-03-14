DROP INDEX IF EXISTS idx_runs_session_idempotency_key;

ALTER TABLE runs
DROP COLUMN IF EXISTS idempotency_key;

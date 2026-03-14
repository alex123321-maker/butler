ALTER TABLE runs
ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_session_idempotency_key
ON runs (session_key, idempotency_key)
WHERE idempotency_key IS NOT NULL;

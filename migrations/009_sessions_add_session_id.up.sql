-- Add a stable UUID-based session_id column separate from the human-readable session_key.
-- Backfill existing rows with a deterministic gen_random_uuid() value.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS session_id UUID NOT NULL DEFAULT gen_random_uuid();

CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_session_id ON sessions (session_id);

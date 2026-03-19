-- Memory management fields for C-01: Memory policy model
-- Adds confirmation_state, effective_status, suppressed, expires_at, edited_by, edited_at

-- Profile memory: editable, confirmable, suppressible
ALTER TABLE memory_profile
    ADD COLUMN IF NOT EXISTS confirmation_state TEXT NOT NULL DEFAULT 'auto_confirmed',
    ADD COLUMN IF NOT EXISTS effective_status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS suppressed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS edited_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ NULL;

COMMENT ON COLUMN memory_profile.confirmation_state IS 'pending, confirmed, rejected, auto_confirmed';
COMMENT ON COLUMN memory_profile.effective_status IS 'active, inactive, suppressed, expired, deleted';
COMMENT ON COLUMN memory_profile.suppressed IS 'soft-suppressed by user, not shown in retrieval but kept for audit';
COMMENT ON COLUMN memory_profile.expires_at IS 'optional expiration time for temporary profile entries';
COMMENT ON COLUMN memory_profile.edited_by IS 'actor who last edited: user, system, pipeline';
COMMENT ON COLUMN memory_profile.edited_at IS 'timestamp of last user/system edit';

CREATE INDEX IF NOT EXISTS idx_memory_profile_confirmation
    ON memory_profile (confirmation_state, effective_status)
    WHERE confirmation_state = 'pending';

CREATE INDEX IF NOT EXISTS idx_memory_profile_effective
    ON memory_profile (scope_type, scope_id, effective_status, suppressed)
    WHERE effective_status = 'active' AND suppressed = FALSE;

-- Episodic memory: confirmable, suppressible (not directly editable content)
ALTER TABLE memory_episodes
    ADD COLUMN IF NOT EXISTS confirmation_state TEXT NOT NULL DEFAULT 'auto_confirmed',
    ADD COLUMN IF NOT EXISTS effective_status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS suppressed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS edited_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ NULL;

COMMENT ON COLUMN memory_episodes.confirmation_state IS 'pending, confirmed, rejected, auto_confirmed';
COMMENT ON COLUMN memory_episodes.effective_status IS 'active, inactive, suppressed, expired, deleted';
COMMENT ON COLUMN memory_episodes.suppressed IS 'soft-suppressed by user, not shown in retrieval but kept for audit';

CREATE INDEX IF NOT EXISTS idx_memory_episodes_confirmation
    ON memory_episodes (confirmation_state, effective_status)
    WHERE confirmation_state = 'pending';

CREATE INDEX IF NOT EXISTS idx_memory_episodes_effective
    ON memory_episodes (scope_type, scope_id, effective_status, suppressed)
    WHERE effective_status = 'active' AND suppressed = FALSE;

-- Chunk memory: suppressible (not confirmable, system-generated)
ALTER TABLE memory_chunks
    ADD COLUMN IF NOT EXISTS confirmation_state TEXT NOT NULL DEFAULT 'auto_confirmed',
    ADD COLUMN IF NOT EXISTS effective_status TEXT NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS suppressed BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS edited_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS edited_at TIMESTAMPTZ NULL;

COMMENT ON COLUMN memory_chunks.effective_status IS 'active, inactive, suppressed, expired, deleted';
COMMENT ON COLUMN memory_chunks.suppressed IS 'soft-suppressed by user, not shown in retrieval but kept for audit';

CREATE INDEX IF NOT EXISTS idx_memory_chunks_effective
    ON memory_chunks (scope_type, scope_id, effective_status, suppressed)
    WHERE effective_status = 'active' AND suppressed = FALSE;

-- Working memory: not editable/confirmable via UI (internal execution state)
-- No changes needed for working memory management fields

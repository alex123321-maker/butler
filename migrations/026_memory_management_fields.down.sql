-- Rollback memory management fields

DROP INDEX IF EXISTS idx_memory_profile_confirmation;
DROP INDEX IF EXISTS idx_memory_profile_effective;

ALTER TABLE memory_profile
    DROP COLUMN IF EXISTS confirmation_state,
    DROP COLUMN IF EXISTS effective_status,
    DROP COLUMN IF EXISTS suppressed,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS edited_by,
    DROP COLUMN IF EXISTS edited_at;

DROP INDEX IF EXISTS idx_memory_episodes_confirmation;
DROP INDEX IF EXISTS idx_memory_episodes_effective;

ALTER TABLE memory_episodes
    DROP COLUMN IF EXISTS confirmation_state,
    DROP COLUMN IF EXISTS effective_status,
    DROP COLUMN IF EXISTS suppressed,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS edited_by,
    DROP COLUMN IF EXISTS edited_at;

DROP INDEX IF EXISTS idx_memory_chunks_effective;

ALTER TABLE memory_chunks
    DROP COLUMN IF EXISTS confirmation_state,
    DROP COLUMN IF EXISTS effective_status,
    DROP COLUMN IF EXISTS suppressed,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS edited_by,
    DROP COLUMN IF EXISTS edited_at;

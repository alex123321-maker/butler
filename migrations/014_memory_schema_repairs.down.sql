DROP INDEX IF EXISTS idx_memory_episodes_embedding_cosine;
DROP INDEX IF EXISTS idx_memory_episodes_scope_status;

ALTER TABLE memory_episodes
    ALTER COLUMN embedding TYPE vector USING embedding::vector,
    DROP COLUMN IF EXISTS confidence,
    DROP COLUMN IF EXISTS memory_type;

DROP INDEX IF EXISTS idx_memory_profile_scope_status;
DROP INDEX IF EXISTS idx_memory_profile_active_scope_key;

DELETE FROM memory_profile WHERE status <> 'active';

ALTER TABLE memory_profile
    DROP COLUMN IF EXISTS confidence,
    DROP COLUMN IF EXISTS memory_type;

ALTER TABLE memory_profile
    ADD CONSTRAINT uq_memory_profile_scope_key UNIQUE (scope_type, scope_id, key);

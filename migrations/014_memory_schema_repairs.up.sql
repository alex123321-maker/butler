ALTER TABLE memory_profile
    ADD COLUMN IF NOT EXISTS memory_type TEXT NOT NULL DEFAULT 'profile',
    ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0;

DROP INDEX IF EXISTS idx_memory_profile_active_scope_key;
ALTER TABLE memory_profile DROP CONSTRAINT IF EXISTS uq_memory_profile_scope_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_profile_active_scope_key
    ON memory_profile (scope_type, scope_id, key)
    WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_memory_profile_scope_status
    ON memory_profile (scope_type, scope_id, status, key);

ALTER TABLE memory_episodes
    ADD COLUMN IF NOT EXISTS memory_type TEXT NOT NULL DEFAULT 'episodic',
    ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'memory_episodes' AND column_name = 'embedding' AND udt_name = 'vector'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'memory_episodes' AND column_name = 'embedding_repaired'
    ) THEN
        ALTER TABLE memory_episodes ADD COLUMN embedding_repaired vector(1536);
        UPDATE memory_episodes
        SET embedding_repaired = embedding::vector(1536)
        WHERE embedding IS NOT NULL AND vector_dims(embedding) = 1536;
        ALTER TABLE memory_episodes DROP COLUMN embedding;
        ALTER TABLE memory_episodes RENAME COLUMN embedding_repaired TO embedding;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_memory_episodes_scope_status
    ON memory_episodes (scope_type, scope_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_episodes_embedding_cosine
    ON memory_episodes USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

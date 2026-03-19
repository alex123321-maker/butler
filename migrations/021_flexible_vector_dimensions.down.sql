-- Down migration: restore vector(1536) columns and IVFFlat indexes.

-- memory_episodes
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'memory_episodes' AND column_name = 'embedding'
    ) THEN
        -- Delete rows with non-1536 embeddings that can't be cast back.
        DELETE FROM memory_episodes WHERE embedding IS NOT NULL AND vector_dims(embedding) <> 1536;

        ALTER TABLE memory_episodes ALTER COLUMN embedding TYPE vector(1536) USING embedding::vector(1536);

        CREATE INDEX IF NOT EXISTS idx_memory_episodes_embedding_cosine
            ON memory_episodes USING ivfflat (embedding vector_cosine_ops)
            WITH (lists = 100);
    END IF;
END $$;

-- memory_chunks
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'memory_chunks' AND column_name = 'embedding'
    ) THEN
        DELETE FROM memory_chunks WHERE embedding IS NOT NULL AND vector_dims(embedding) <> 1536;

        ALTER TABLE memory_chunks ALTER COLUMN embedding TYPE vector(1536) USING embedding::vector(1536);

        CREATE INDEX IF NOT EXISTS idx_memory_chunks_embedding_cosine
            ON memory_chunks USING ivfflat (embedding vector_cosine_ops)
            WITH (lists = 100);
    END IF;
END $$;

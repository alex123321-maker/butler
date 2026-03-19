-- Migration 021: Make vector columns dimensionless to support multiple
-- embedding providers (OpenAI 1536d, Ollama nomic-embed-text 768d, etc.).
-- IVFFlat indexes require fixed dimensions, so we drop and recreate them
-- as HNSW indexes which support variable-dimension vectors.

-- memory_episodes: change embedding from vector(1536) to vector (untyped)
-- and rebuild the index.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'memory_episodes' AND column_name = 'embedding'
    ) THEN
        -- Drop the old IVFFlat index.
        DROP INDEX IF EXISTS idx_memory_episodes_embedding_cosine;

        -- The column was vector(1536) with NOT NULL removed in migration 014.
        -- Change to untyped vector so any dimension works.
        ALTER TABLE memory_episodes ALTER COLUMN embedding TYPE vector USING embedding::vector;
    END IF;
END $$;

-- memory_chunks: change embedding from VECTOR(1536) to vector (untyped)
-- and rebuild the index.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'memory_chunks' AND column_name = 'embedding'
    ) THEN
        DROP INDEX IF EXISTS idx_memory_chunks_embedding_cosine;

        ALTER TABLE memory_chunks ALTER COLUMN embedding TYPE vector USING embedding::vector;
    END IF;
END $$;

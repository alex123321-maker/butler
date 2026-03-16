CREATE TABLE IF NOT EXISTS memory_chunks (
    id BIGSERIAL PRIMARY KEY,
    memory_type TEXT NOT NULL DEFAULT 'chunk',
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    provenance JSONB NOT NULL DEFAULT '{}'::jsonb,
    tags JSONB NOT NULL DEFAULT '[]'::jsonb,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    status TEXT NOT NULL DEFAULT 'active',
    embedding VECTOR(1536),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_chunks_scope
    ON memory_chunks (scope_type, scope_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_memory_chunks_source
    ON memory_chunks (source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_memory_chunks_embedding_cosine
    ON memory_chunks USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);

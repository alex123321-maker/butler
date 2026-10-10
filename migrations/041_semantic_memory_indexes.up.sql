-- Retrieval derivatives only; durable memory_chunks remains source of truth.
CREATE TABLE semantic_index_generations (
    id TEXT PRIMARY KEY,
    space_id TEXT NOT NULL,
    space_data JSONB NOT NULL,
    dimensions INTEGER NOT NULL CHECK (dimensions IN (256,768)),
    scope_type TEXT NOT NULL CHECK (scope_type IN ('session','user','global')),
    scope_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'building' CHECK(state IN ('building','ready')),
    quality_proof JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    verified_at TIMESTAMPTZ,
    UNIQUE(id,dimensions),
    UNIQUE(id,scope_type,scope_id)
);
CREATE TABLE semantic_index_entries (
    generation_id TEXT NOT NULL,
    chunk_id BIGINT NOT NULL REFERENCES memory_chunks(id) ON DELETE CASCADE,
    dimensions INTEGER NOT NULL,
    source_revision TIMESTAMPTZ NOT NULL,
    source_hash TEXT NOT NULL CHECK(length(source_hash)=64),
    embedding_hash TEXT NOT NULL CHECK(length(embedding_hash)=64),
    embedding VECTOR NOT NULL,
    indexed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(generation_id,chunk_id),
    FOREIGN KEY(generation_id,dimensions) REFERENCES semantic_index_generations(id,dimensions) ON DELETE CASCADE,
    CHECK(vector_dims(embedding)=dimensions)
);
CREATE TABLE semantic_index_active (
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    generation_id TEXT NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY(scope_type,scope_id),
    FOREIGN KEY(generation_id,scope_type,scope_id) REFERENCES semantic_index_generations(id,scope_type,scope_id)
);
CREATE INDEX semantic_index_entries_source ON semantic_index_entries(chunk_id);

ALTER TABLE memory_working
    ADD COLUMN IF NOT EXISTS memory_type TEXT NOT NULL DEFAULT 'working',
    ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS source_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provenance JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_memory_working_source
    ON memory_working (source_type, source_id);

ALTER TABLE memory_profile
    ADD COLUMN IF NOT EXISTS provenance JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_memory_profile_source
    ON memory_profile (source_type, source_id);

ALTER TABLE memory_episodes
    ADD COLUMN IF NOT EXISTS provenance JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_memory_episodes_source
    ON memory_episodes (source_type, source_id);

CREATE TABLE IF NOT EXISTS memory_links (
    id BIGSERIAL PRIMARY KEY,
    source_memory_type TEXT NOT NULL,
    source_memory_id BIGINT NOT NULL,
    link_type TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_memory_links_source_target UNIQUE (source_memory_type, source_memory_id, link_type, target_type, target_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_links_source
    ON memory_links (source_memory_type, source_memory_id, link_type);

CREATE INDEX IF NOT EXISTS idx_memory_links_target
    ON memory_links (target_type, target_id, link_type);

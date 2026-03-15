CREATE TABLE IF NOT EXISTS memory_profile (
    id BIGSERIAL PRIMARY KEY,
    scope_type TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    key TEXT NOT NULL,
    value_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    summary TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active',
    effective_from TIMESTAMPTZ NULL,
    effective_to TIMESTAMPTZ NULL,
    supersedes_id BIGINT NULL REFERENCES memory_profile (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_memory_profile_scope_key UNIQUE (scope_type, scope_id, key)
);

CREATE INDEX IF NOT EXISTS idx_memory_profile_scope ON memory_profile (scope_type, scope_id);

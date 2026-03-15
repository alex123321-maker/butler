DROP INDEX IF EXISTS idx_memory_links_target;
DROP INDEX IF EXISTS idx_memory_links_source;
DROP TABLE IF EXISTS memory_links;

DROP INDEX IF EXISTS idx_memory_episodes_source;
ALTER TABLE memory_episodes
    DROP COLUMN IF EXISTS provenance;

DROP INDEX IF EXISTS idx_memory_profile_source;
ALTER TABLE memory_profile
    DROP COLUMN IF EXISTS provenance;

DROP INDEX IF EXISTS idx_memory_working_source;
ALTER TABLE memory_working
    DROP COLUMN IF EXISTS provenance,
    DROP COLUMN IF EXISTS source_id,
    DROP COLUMN IF EXISTS source_type,
    DROP COLUMN IF EXISTS memory_type;

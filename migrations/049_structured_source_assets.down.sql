DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_source_assets) OR EXISTS(SELECT 1 FROM memory_index_jobs) OR EXISTS(SELECT 1 FROM semantic_index_generations WHERE scope_type IN ('project','task')) THEN
  RAISE EXCEPTION 'refusing to discard durable original assets or owned index history';
 END IF;
END $$;
DROP INDEX IF EXISTS memory_chunks_locator_fts;
DROP TABLE IF EXISTS memory_index_jobs;
DROP TRIGGER IF EXISTS memory_redact_assets ON memory_sources;
DROP FUNCTION IF EXISTS butler_memory_redact_assets();
DROP TABLE IF EXISTS memory_source_assets;
ALTER TABLE semantic_index_generations DROP CONSTRAINT semantic_index_generations_scope_type_check;
ALTER TABLE semantic_index_generations ADD CONSTRAINT semantic_index_generations_scope_type_check CHECK(scope_type IN ('session','user','global'));

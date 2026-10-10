ALTER TABLE memory_object_sources ADD COLUMN IF NOT EXISTS fragment_ordinal INT NOT NULL DEFAULT 0;
ALTER TABLE semantic_index_generations DROP CONSTRAINT semantic_index_generations_scope_type_check;
ALTER TABLE semantic_index_generations ADD CONSTRAINT semantic_index_generations_scope_type_check CHECK(scope_type IN ('session','user','global','project','task'));
CREATE TABLE memory_source_assets (
 source_id TEXT NOT NULL, source_revision BIGINT NOT NULL,
 format TEXT NOT NULL, path TEXT NOT NULL, repository_revision TEXT NOT NULL DEFAULT '',
 original_hash TEXT NOT NULL CHECK(length(original_hash)=64), original_bytes BYTEA,
 parser_version TEXT NOT NULL, method TEXT NOT NULL DEFAULT '', uncertain BOOLEAN NOT NULL DEFAULT FALSE,
 status TEXT NOT NULL CHECK(status IN ('parsed','ready','failed','deleted')), error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(source_id,source_revision),
 FOREIGN KEY(source_id,source_revision) REFERENCES memory_source_revisions(source_id,revision)
);
CREATE FUNCTION butler_memory_redact_assets() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status IN ('deleted','revoked') THEN
  UPDATE memory_source_assets SET original_bytes=NULL,path='',repository_revision='',status='deleted',error_code='' WHERE source_id=NEW.source_id;
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER memory_redact_assets AFTER UPDATE OF status ON memory_sources FOR EACH ROW EXECUTE FUNCTION butler_memory_redact_assets();

CREATE TABLE memory_index_jobs (
 job_id TEXT PRIMARY KEY REFERENCES memory_aux_jobs(job_id),
 generation_id TEXT NOT NULL REFERENCES semantic_index_generations(id),
 scope_type TEXT NOT NULL,scope_id TEXT NOT NULL,max_chunks INT NOT NULL CHECK(max_chunks BETWEEN 1 AND 10000)
);
CREATE INDEX memory_chunks_locator_fts ON memory_chunks USING GIN ((to_tsvector('simple'::regconfig,coalesce(title,'')||' '||coalesce(content,'')||' '||coalesce(source_id,'')||' '||coalesce(provenance->'locator'->>'path','')||' '||coalesce(provenance->'locator'->>'repository_revision',''))||to_tsvector('russian'::regconfig,coalesce(title,'')||' '||coalesce(content,'')||' '||coalesce(source_id,'')||' '||coalesce(provenance->'locator'->>'path','')||' '||coalesce(provenance->'locator'->>'repository_revision',''))));

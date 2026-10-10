-- Source identity is distinct from retrieval relevance and owner authority.
CREATE TABLE memory_sources (
 source_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL,
 source_kind TEXT NOT NULL CHECK(source_kind IN ('message','document','code','owner_statement','model_candidate','legacy')),
 scope_type TEXT NOT NULL, scope_id TEXT NOT NULL,
 external_key TEXT NOT NULL, head_revision BIGINT NOT NULL DEFAULT 1,
 access_revision BIGINT NOT NULL DEFAULT 1,
 status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked','deleted','quarantined')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(owner_id,external_key)
);
CREATE TABLE memory_source_revisions (
 source_id TEXT NOT NULL REFERENCES memory_sources(source_id), revision BIGINT NOT NULL,
 content_hash TEXT NOT NULL CHECK(length(content_hash)=64), title TEXT NOT NULL, body TEXT NOT NULL,
 locator JSONB NOT NULL DEFAULT '{}', origin JSONB NOT NULL, author TEXT NOT NULL,
 event_time TIMESTAMPTZ, extracted_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(source_id,revision)
);
CREATE TABLE memory_source_dependencies (
 child_source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 parent_source_id TEXT NOT NULL REFERENCES memory_sources(source_id), parent_revision BIGINT NOT NULL,
 PRIMARY KEY(child_source_id,parent_source_id), CHECK(child_source_id<>parent_source_id)
);
CREATE INDEX memory_source_parent ON memory_source_dependencies(parent_source_id);
CREATE TABLE memory_object_sources (
 memory_type TEXT NOT NULL CHECK(memory_type IN ('profile','episodic','chunk')),
 object_id BIGINT NOT NULL, source_id TEXT NOT NULL REFERENCES memory_sources(source_id), source_revision BIGINT NOT NULL,
 fragment_key TEXT NOT NULL DEFAULT '', fragment_hash TEXT NOT NULL DEFAULT '', locator JSONB NOT NULL DEFAULT '{}',
 PRIMARY KEY(memory_type,object_id)
);
CREATE INDEX memory_objects_source ON memory_object_sources(source_id);
CREATE TABLE memory_fact_heads (
 owner_id TEXT NOT NULL, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL, fact_key TEXT NOT NULL,
 active_claim_id TEXT NOT NULL DEFAULT '', revision BIGINT NOT NULL DEFAULT 0,
 PRIMARY KEY(owner_id,scope_type,scope_id,fact_key)
);
CREATE TABLE memory_claims (
 claim_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, scope_type TEXT NOT NULL, scope_id TEXT NOT NULL, fact_key TEXT NOT NULL,
 source_id TEXT NOT NULL REFERENCES memory_sources(source_id), source_revision BIGINT NOT NULL,
 profile_id BIGINT REFERENCES memory_profile(id) ON DELETE SET NULL,
 value_json JSONB NOT NULL, summary TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'candidate' CHECK(status IN ('candidate','confirmed','superseded','rejected','deleted')),
 extraction_confidence DOUBLE PRECISION NOT NULL CHECK(extraction_confidence>=0 AND extraction_confidence<=1),
 supersedes_id TEXT NOT NULL DEFAULT '', accepted_by TEXT NOT NULL DEFAULT '',
 event_time TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), accepted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX memory_one_confirmed_claim ON memory_claims(owner_id,scope_type,scope_id,fact_key) WHERE status='confirmed';
CREATE TABLE memory_scope_grants (
 task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE, source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 task_revision BIGINT NOT NULL, access_revision BIGINT NOT NULL, purpose TEXT NOT NULL, owner_id TEXT NOT NULL,
 PRIMARY KEY(task_id,source_id)
);
CREATE TABLE memory_deletions (
 deletion_id TEXT PRIMARY KEY, source_id TEXT NOT NULL, owner_id TEXT NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('delete','revoke')), watermark BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), cache_status TEXT NOT NULL DEFAULT 'pending'
);
CREATE TABLE memory_restore_state (singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(singleton), watermark BIGINT NOT NULL DEFAULT 0);
INSERT INTO memory_restore_state(singleton) VALUES(TRUE);

-- Legacy values remain inspectable, with explicit uncertainty. An old automatic
-- confirmation is not evidence that an authenticated owner accepted the claim.
INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key,status)
SELECT 'legacy:profile:'||p.id, COALESCE(CASE WHEN p.scope_type='user' THEN p.scope_id ELSE s.user_id END,'legacy-unassigned'),
 'legacy',p.scope_type,p.scope_id,'legacy:profile:'||p.id,'quarantined'
FROM memory_profile p LEFT JOIN sessions s ON p.scope_type='session' AND s.session_key=p.scope_id;
INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key,status)
SELECT 'legacy:episodic:'||p.id, COALESCE(CASE WHEN p.scope_type='user' THEN p.scope_id ELSE s.user_id END,'legacy-unassigned'),
 'legacy',p.scope_type,p.scope_id,'legacy:episodic:'||p.id,'quarantined'
FROM memory_episodes p LEFT JOIN sessions s ON p.scope_type='session' AND s.session_key=p.scope_id;
INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key,status)
SELECT 'legacy:chunk:'||p.id, COALESCE(CASE WHEN p.scope_type='user' THEN p.scope_id ELSE s.user_id END,'legacy-unassigned'),
 'legacy',p.scope_type,p.scope_id,'legacy:chunk:'||p.id,'quarantined'
FROM memory_chunks p LEFT JOIN sessions s ON p.scope_type='session' AND s.session_key=p.scope_id;
INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,origin,author,event_time)
SELECT 'legacy:profile:'||id,1,repeat('0',64),key,value_json::text,jsonb_build_object('kind','legacy_unknown','sensitivity','private','source_refs',jsonb_build_array(source_type||':'||source_id),'parent_refs',COALESCE(provenance->'parent_refs','[]')),'unknown',created_at FROM memory_profile
UNION ALL SELECT 'legacy:episodic:'||id,1,repeat('0',64),summary,content,jsonb_build_object('kind','legacy_unknown','sensitivity','private','source_refs',jsonb_build_array(source_type||':'||source_id),'parent_refs',COALESCE(provenance->'parent_refs','[]')),'unknown',created_at FROM memory_episodes
UNION ALL SELECT 'legacy:chunk:'||id,1,repeat('0',64),title,content,jsonb_build_object('kind','legacy_unknown','sensitivity','private','source_refs',jsonb_build_array(source_type||':'||source_id),'parent_refs',COALESCE(provenance->'parent_refs','[]')),'unknown',created_at FROM memory_chunks;
INSERT INTO memory_object_sources(memory_type,object_id,source_id,source_revision)
SELECT 'profile',id,'legacy:profile:'||id,1 FROM memory_profile UNION ALL SELECT 'episodic',id,'legacy:episodic:'||id,1 FROM memory_episodes UNION ALL SELECT 'chunk',id,'legacy:chunk:'||id,1 FROM memory_chunks;

-- Database publication fence applies even to older stores/pipelines. Only a
-- catalog-bound current source may publish retrieval data. Ordinary legacy
-- writes are retained as quarantined candidate data for migration/review.
CREATE FUNCTION butler_memory_source_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE src memory_sources; typ TEXT; sid TEXT;
BEGIN
 typ:=CASE TG_TABLE_NAME WHEN 'memory_profile' THEN 'profile' WHEN 'memory_episodes' THEN 'episodic' ELSE 'chunk' END;
 IF NEW.source_type='catalog' THEN
  SELECT * INTO src FROM memory_sources WHERE source_id=NEW.source_id FOR SHARE;
  IF NOT FOUND OR src.status<>'active' OR src.scope_type<>NEW.scope_type OR src.scope_id<>NEW.scope_id THEN
   RAISE EXCEPTION 'memory source is stale or inaccessible' USING ERRCODE='40001';
  END IF;
 ELSE
  NEW.confirmation_state:='pending';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER memory_profile_source_fence BEFORE INSERT OR UPDATE ON memory_profile FOR EACH ROW EXECUTE FUNCTION butler_memory_source_fence();
CREATE TRIGGER memory_episodes_source_fence BEFORE INSERT OR UPDATE ON memory_episodes FOR EACH ROW EXECUTE FUNCTION butler_memory_source_fence();
CREATE TRIGGER memory_chunks_source_fence BEFORE INSERT OR UPDATE ON memory_chunks FOR EACH ROW EXECUTE FUNCTION butler_memory_source_fence();
CREATE FUNCTION butler_memory_object_link() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE typ TEXT; src memory_sources;
BEGIN
 typ:=CASE TG_TABLE_NAME WHEN 'memory_profile' THEN 'profile' WHEN 'memory_episodes' THEN 'episodic' ELSE 'chunk' END;
 IF NEW.source_type='catalog' THEN
  SELECT * INTO src FROM memory_sources WHERE source_id=NEW.source_id;
  INSERT INTO memory_object_sources(memory_type,object_id,source_id,source_revision) VALUES(typ,NEW.id,src.source_id,src.head_revision)
   ON CONFLICT(memory_type,object_id) DO UPDATE SET source_id=EXCLUDED.source_id,source_revision=EXCLUDED.source_revision;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER memory_profile_object_link AFTER INSERT OR UPDATE ON memory_profile FOR EACH ROW EXECUTE FUNCTION butler_memory_object_link();
CREATE TRIGGER memory_episodes_object_link AFTER INSERT OR UPDATE ON memory_episodes FOR EACH ROW EXECUTE FUNCTION butler_memory_object_link();
CREATE TRIGGER memory_chunks_object_link AFTER INSERT OR UPDATE ON memory_chunks FOR EACH ROW EXECUTE FUNCTION butler_memory_object_link();

CREATE FUNCTION butler_memory_source_visible(typ TEXT,oid BIGINT) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 SELECT EXISTS(SELECT 1 FROM memory_object_sources o JOIN memory_sources s USING(source_id)
  WHERE o.memory_type=typ AND o.object_id=oid AND s.status='active' AND s.head_revision=o.source_revision
   AND NOT EXISTS(SELECT 1 FROM memory_source_dependencies d JOIN memory_sources p ON p.source_id=d.parent_source_id WHERE d.child_source_id=s.source_id AND (p.status<>'active' OR p.head_revision<>d.parent_revision)))
$$;

ALTER TABLE memory_deletions ADD COLUMN cache_cursor_session TEXT NOT NULL DEFAULT '', ADD COLUMN cache_cursor_run TEXT NOT NULL DEFAULT '';
CREATE TABLE memory_run_sources (
 run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
 source_id TEXT NOT NULL REFERENCES memory_sources(source_id), source_revision BIGINT NOT NULL,
 access_revision BIGINT NOT NULL, content_hash TEXT NOT NULL,
 PRIMARY KEY(run_id,source_id)
);
CREATE TABLE memory_run_transcripts (
 run_id TEXT PRIMARY KEY REFERENCES runs(run_id) ON DELETE CASCADE,
 session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE, source_revision BIGINT NOT NULL,
 deletion_watermark BIGINT NOT NULL
);
CREATE FUNCTION butler_memory_check_run_sources(rid TEXT) RETURNS VOID LANGUAGE plpgsql AS $$
DECLARE p RECORD;
BEGIN
 FOR p IN SELECT h.source_revision,t.source_revision AS pinned_revision FROM memory_run_transcripts t JOIN transcript_heads h USING(session_key) WHERE t.run_id=rid FOR SHARE OF h LOOP
  IF p.source_revision<>p.pinned_revision THEN RAISE EXCEPTION 'run transcript was redacted or corrected' USING ERRCODE='40001'; END IF;
 END LOOP;
 FOR p IN SELECT s.source_id,s.status,s.head_revision,s.access_revision,r.content_hash AS actual_hash,b.source_revision,b.access_revision AS pinned_access,b.content_hash AS pinned_hash
 FROM memory_run_sources b JOIN memory_sources s USING(source_id) JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision
 WHERE b.run_id=rid ORDER BY s.source_id FOR SHARE OF s LOOP
  IF p.status<>'active' OR p.head_revision<>p.source_revision OR p.access_revision<>p.pinned_access OR p.actual_hash<>p.pinned_hash THEN
   RAISE EXCEPTION 'run memory source is stale or deleted' USING ERRCODE='40001';
  END IF;
 END LOOP;
 -- Working/legacy derivatives can lack exact source edges. A deletion also
 -- invalidates every previously prepared context; fresh runs pin the new head.
 FOR p IN SELECT t.deletion_watermark,s.watermark FROM memory_run_transcripts t CROSS JOIN memory_restore_state s WHERE t.run_id=rid FOR SHARE OF s LOOP
  IF p.deletion_watermark<>p.watermark THEN RAISE EXCEPTION 'run memory deletion watermark changed' USING ERRCODE='40001'; END IF;
 END LOOP;
END $$;
CREATE FUNCTION butler_memory_publication_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='messages' THEN
  -- A fresh owner correction/stop remains receivable even when an old model
  -- context is invalid. It is data, not late model publication.
  IF NEW.role='user' THEN RETURN NEW; END IF;
 END IF;
 PERFORM butler_memory_check_run_sources(NEW.run_id); RETURN NEW;
END $$;
CREATE TRIGGER memory_final_publication_fence BEFORE INSERT ON run_final_results FOR EACH ROW EXECUTE FUNCTION butler_memory_publication_fence();
CREATE TRIGGER memory_artifact_publication_fence BEFORE INSERT ON artifacts FOR EACH ROW EXECUTE FUNCTION butler_memory_publication_fence();
CREATE TRIGGER memory_message_publication_fence BEFORE INSERT ON messages FOR EACH ROW EXECUTE FUNCTION butler_memory_publication_fence();
CREATE TRIGGER memory_working_publication_fence BEFORE INSERT OR UPDATE ON memory_working FOR EACH ROW EXECUTE FUNCTION butler_memory_publication_fence();
CREATE FUNCTION butler_memory_task_working_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.working_json IS DISTINCT FROM OLD.working_json AND NOT EXISTS(SELECT 1 FROM memory_deletions WHERE deletion_id=current_setting('butler.memory_redaction',TRUE) AND owner_id=OLD.owner_id) THEN
  PERFORM butler_memory_check_run_sources(NEW.last_run_id);
 END IF; RETURN NEW;
END $$;
CREATE TRIGGER memory_task_working_fence BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION butler_memory_task_working_fence();

-- Messages have durable provenance even if no extraction job ever runs.
UPDATE memory_source_revisions r SET content_hash=encode(sha256(convert_to(r.title||r.body||r.origin::text,'UTF8')),'hex') FROM memory_sources s WHERE s.source_id=r.source_id AND s.source_kind='legacy' AND r.content_hash=repeat('0',64);
INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key)
SELECT 'message:'||m.message_id,s.user_id,'message','session',m.session_key,'message:'||m.message_id FROM messages m JOIN sessions s USING(session_key);
INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author,event_time)
SELECT 'message:'||m.message_id,1,encode(sha256(convert_to(m.content||m.metadata::text,'UTF8')),'hex'),m.role,m.content,jsonb_build_object('message_id',m.message_id,'session_key',m.session_key,'sequence',m.message_sequence),
 jsonb_build_object('kind','memory_derived','sensitivity',COALESCE(m.metadata->'butler_provenance'->>'sensitivity','private'),'source_refs',jsonb_build_array('message:'||m.message_id),'parent_refs',COALESCE(m.metadata->'butler_provenance'->'parent_refs','[]')),
 CASE WHEN m.role='user' THEN s.user_id ELSE m.role END,m.created_at FROM messages m JOIN sessions s USING(session_key);
INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision)
SELECT o.source_id,'message:'||m.message_id,1 FROM memory_object_sources o
JOIN memory_sources s ON s.source_id=o.source_id
JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision
JOIN messages m ON r.origin->'source_refs' ? ('message:'||m.message_id)
ON CONFLICT DO NOTHING;
INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision)
SELECT 'message:'||m.message_id,'message:'||p.message_id,1 FROM messages m JOIN messages p ON p.session_key=m.session_key AND p.message_sequence<m.message_sequence
 AND p.message_sequence>=COALESCE((SELECT MAX(x.message_sequence) FROM messages x WHERE x.session_key=m.session_key AND x.role<>'user' AND x.message_sequence<m.message_sequence),0)
WHERE m.role<>'user' ON CONFLICT DO NOTHING;

CREATE FUNCTION butler_memory_register_message() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE own TEXT; sid TEXT; parent RECORD;
BEGIN
 SELECT user_id INTO own FROM sessions WHERE session_key=NEW.session_key;
 sid:='message:'||NEW.message_id;
 INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key) VALUES(sid,own,'message','session',NEW.session_key,sid);
 INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author,event_time)
 VALUES(sid,1,encode(sha256(convert_to(NEW.content||NEW.metadata::text,'UTF8')),'hex'),NEW.role,NEW.content,jsonb_build_object('message_id',NEW.message_id,'session_key',NEW.session_key,'sequence',NEW.message_sequence),
 jsonb_build_object('kind','memory_derived','sensitivity',COALESCE(NEW.metadata->'butler_provenance'->>'sensitivity','private'),'source_refs',jsonb_build_array(sid),'parent_refs',COALESCE(NEW.metadata->'butler_provenance'->'parent_refs','[]')),
 CASE WHEN NEW.role='user' THEN own ELSE NEW.role END,NEW.created_at);
 IF NEW.role<>'user' THEN
  INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision)
  SELECT sid,'message:'||m.message_id,1 FROM messages m WHERE m.session_key=NEW.session_key AND m.message_sequence<NEW.message_sequence
   AND m.message_sequence>=COALESCE((SELECT MAX(x.message_sequence) FROM messages x WHERE x.session_key=NEW.session_key AND x.role<>'user' AND x.message_sequence<NEW.message_sequence),0) ON CONFLICT DO NOTHING;
  INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision)
  SELECT sid,source_id,source_revision FROM memory_run_sources WHERE run_id=NEW.run_id AND source_id<>sid ON CONFLICT DO NOTHING;
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER memory_register_message AFTER INSERT ON messages FOR EACH ROW EXECUTE FUNCTION butler_memory_register_message();

ALTER TABLE run_final_results ADD COLUMN privacy_redacted BOOLEAN NOT NULL DEFAULT FALSE, ADD COLUMN redaction_id TEXT NOT NULL DEFAULT '';
ALTER TABLE artifacts ADD COLUMN privacy_redacted BOOLEAN NOT NULL DEFAULT FALSE, ADD COLUMN redaction_id TEXT NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN privacy_redacted BOOLEAN NOT NULL DEFAULT FALSE, ADD COLUMN redaction_id TEXT NOT NULL DEFAULT '';
ALTER TABLE delivery_outbox ADD COLUMN privacy_redacted BOOLEAN NOT NULL DEFAULT FALSE, ADD COLUMN redaction_id TEXT NOT NULL DEFAULT '';
ALTER TABLE task_results ADD COLUMN privacy_redacted BOOLEAN NOT NULL DEFAULT FALSE, ADD COLUMN redaction_id TEXT NOT NULL DEFAULT '';
CREATE FUNCTION butler_memory_update_message_source() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE rev BIGINT; sid TEXT;
BEGIN
 IF NEW.privacy_redacted OR (NEW.content,NEW.metadata) IS NOT DISTINCT FROM (OLD.content,OLD.metadata) THEN RETURN NULL; END IF;
 sid:='message:'||NEW.message_id;
 UPDATE memory_sources SET head_revision=head_revision+1,updated_at=clock_timestamp() WHERE source_id=sid RETURNING head_revision INTO rev;
 INSERT INTO memory_source_revisions(source_id,revision,content_hash,title,body,locator,origin,author,event_time)
 SELECT sid,rev,encode(sha256(convert_to(NEW.content||NEW.metadata::text,'UTF8')),'hex'),NEW.role,NEW.content,locator,
 jsonb_build_object('kind','memory_derived','sensitivity',COALESCE(NEW.metadata->'butler_provenance'->>'sensitivity','private'),'source_refs',jsonb_build_array(sid)),author,NEW.created_at
 FROM memory_source_revisions WHERE source_id=sid AND revision=rev-1;
 RETURN NULL;
END $$;
CREATE TRIGGER memory_update_message_source AFTER UPDATE ON messages FOR EACH ROW EXECUTE FUNCTION butler_memory_update_message_source();

CREATE FUNCTION butler_memory_redaction_authorized(key TEXT,did TEXT) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 SELECT did<>'' AND current_setting('butler.memory_redaction',TRUE)=did
 AND EXISTS(SELECT 1 FROM memory_deletions d JOIN sessions s ON s.user_id=d.owner_id WHERE d.deletion_id=did AND s.session_key=key)
$$;
CREATE OR REPLACE FUNCTION butler_immutable_final_result() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.privacy_redacted AND NOT OLD.privacy_redacted AND NEW.content='' AND butler_memory_redaction_authorized(OLD.session_key,NEW.redaction_id)
 AND (to_jsonb(NEW)-ARRAY['content','privacy_redacted','redaction_id'])=(to_jsonb(OLD)-ARRAY['content','privacy_redacted','redaction_id']) THEN RETURN NEW; END IF;
 RAISE EXCEPTION 'final result is immutable';
END $$;
CREATE OR REPLACE FUNCTION butler_protect_final_components() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='artifacts' THEN
  IF NEW.privacy_redacted AND NOT OLD.privacy_redacted AND NEW.content_text='' AND NEW.content_json='{}' AND NEW.title='' AND NEW.summary='' AND butler_memory_redaction_authorized(OLD.session_key,NEW.redaction_id)
   AND (to_jsonb(NEW)-ARRAY['content_text','content_json','title','summary','privacy_redacted','redaction_id','updated_at'])=(to_jsonb(OLD)-ARRAY['content_text','content_json','title','summary','privacy_redacted','redaction_id','updated_at']) THEN RETURN NEW; END IF;
  IF EXISTS(SELECT 1 FROM run_final_results WHERE artifact_id=OLD.artifact_id) THEN RAISE EXCEPTION 'final artifact is immutable'; END IF;
 ELSE
  IF NEW.privacy_redacted AND NOT OLD.privacy_redacted AND NEW.content='' AND NEW.metadata->>'privacy_redacted'='true' AND butler_memory_redaction_authorized(OLD.session_key,NEW.redaction_id)
   AND (to_jsonb(NEW)-ARRAY['content','metadata','privacy_redacted','redaction_id'])=(to_jsonb(OLD)-ARRAY['content','metadata','privacy_redacted','redaction_id']) THEN RETURN NEW; END IF;
  IF EXISTS(SELECT 1 FROM run_final_results WHERE message_id=OLD.message_id) THEN RAISE EXCEPTION 'final transcript is immutable'; END IF;
 END IF;RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION butler_immutable_notification() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.privacy_redacted AND NOT OLD.privacy_redacted AND NEW.content='' AND butler_memory_redaction_authorized(OLD.session_key,NEW.redaction_id)
  AND (to_jsonb(NEW)-ARRAY['content','privacy_redacted','redaction_id','status','error_code','claim_id','claim_until','updated_at'])=(to_jsonb(OLD)-ARRAY['content','privacy_redacted','redaction_id','status','error_code','claim_id','claim_until','updated_at']) THEN RETURN NEW; END IF;
 IF NEW.notification_id IS DISTINCT FROM OLD.notification_id OR NEW.result_id IS DISTINCT FROM OLD.result_id OR NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.session_key IS DISTINCT FROM OLD.session_key OR NEW.channel IS DISTINCT FROM OLD.channel OR NEW.chunk_index IS DISTINCT FROM OLD.chunk_index OR NEW.content IS DISTINCT FROM OLD.content OR NEW.deadline_at IS DISTINCT FROM OLD.deadline_at OR NEW.privacy_redacted IS DISTINCT FROM OLD.privacy_redacted OR NEW.redaction_id IS DISTINCT FROM OLD.redaction_id THEN RAISE EXCEPTION 'notification payload is immutable'; END IF;RETURN NEW;
END $$;

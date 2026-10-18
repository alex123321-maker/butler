-- Content removal must survive stale observations after an earlier deletion.
CREATE OR REPLACE FUNCTION butler_memory_experience_publication_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE rid TEXT; did TEXT;
BEGIN
 IF TG_TABLE_NAME='messages' THEN IF NEW.role='user' THEN RETURN NEW; END IF; END IF;
 IF TG_TABLE_NAME='tasks' AND TG_OP='UPDATE' THEN
  did:=current_setting('butler.memory_redaction',TRUE);
  IF COALESCE(did,'')<>'' THEN
   IF NEW.working_json=jsonb_build_object('plan','[]'::jsonb,'pending_actions','[]'::jsonb,'scratch',jsonb_build_object('privacy_redacted',TRUE,'deletion_id',did))
    AND NEW.working_revision=OLD.working_revision+1
    AND (to_jsonb(NEW)-ARRAY['working_json','working_revision','updated_at'])=(to_jsonb(OLD)-ARRAY['working_json','working_revision','updated_at'])
    AND butler_memory_redaction_authorized(OLD.session_key,did) THEN RETURN NEW; END IF;
   RAISE EXCEPTION 'experience working erasure requires exact owning deletion' USING ERRCODE='40001';
  END IF;
 END IF;
 IF TG_TABLE_NAME='tasks' THEN rid:=NEW.last_run_id; ELSE rid:=NEW.run_id; END IF;
 IF NOT butler_memory_experience_sources_current(ARRAY(SELECT source_id FROM memory_run_sources WHERE run_id=rid)) THEN
  RAISE EXCEPTION 'run experience tool observations changed' USING ERRCODE='40001';
 END IF;
 RETURN NEW;
END $$;

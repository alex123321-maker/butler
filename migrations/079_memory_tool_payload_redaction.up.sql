-- Durable content-free tombstone for cached payloads of a source-dependent run.
-- Effect journal, exact action hashes and reservations are deliberately untouched.
CREATE TABLE memory_run_payload_redactions (
 run_id TEXT PRIMARY KEY,
 redaction_id TEXT NOT NULL REFERENCES memory_deletions(deletion_id)
);
CREATE FUNCTION butler_memory_payload_redaction_identity() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE sid TEXT;
BEGIN
 IF TG_OP<>'INSERT' THEN RAISE EXCEPTION 'run payload redaction is irreversible'; END IF;
 SELECT session_key INTO sid FROM runs WHERE run_id=NEW.run_id;
 IF sid IS NULL OR NOT butler_memory_redaction_authorized(sid,NEW.redaction_id) THEN
  RAISE EXCEPTION 'run payload redaction requires owning deletion';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER memory_payload_redaction_identity BEFORE INSERT OR UPDATE OR DELETE ON memory_run_payload_redactions
FOR EACH ROW EXECUTE FUNCTION butler_memory_payload_redaction_identity();

CREATE FUNCTION butler_memory_tool_payload_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE did TEXT;
BEGIN
 IF TG_OP='UPDATE' AND OLD.run_id IS DISTINCT FROM NEW.run_id AND EXISTS(SELECT 1 FROM memory_run_payload_redactions WHERE run_id=OLD.run_id) THEN
  RAISE EXCEPTION 'redacted payload identity is immutable';
 END IF;
 SELECT redaction_id INTO did FROM memory_run_payload_redactions WHERE run_id=NEW.run_id;
 IF did IS NULL THEN RETURN NEW; END IF;
 IF TG_TABLE_NAME='tool_calls' THEN
  IF NEW.args<>'{}'::jsonb OR NEW.result IS DISTINCT FROM jsonb_build_object('privacy_redacted',TRUE,'deletion_id',did) OR COALESCE(NEW.error,'{}'::jsonb)<>'{}'::jsonb THEN
   RAISE EXCEPTION 'redacted tool payload cannot be restored';
  END IF;
 ELSIF TG_TABLE_NAME='approvals' THEN
  IF NEW.args_json<>'{}'::jsonb OR NEW.summary<>'' OR NEW.details_json<>'{}'::jsonb OR NEW.resolution_reason<>'privacy_redacted' THEN
   RAISE EXCEPTION 'redacted approval payload cannot be restored';
  END IF;
 ELSE
  IF NEW.metadata_json IS DISTINCT FROM jsonb_build_object('privacy_redacted',TRUE,'deletion_id',did) OR NEW.reason<>'privacy_redacted' THEN
   RAISE EXCEPTION 'redacted approval event payload cannot be restored';
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER memory_tool_payload_fence BEFORE INSERT OR UPDATE ON tool_calls FOR EACH ROW EXECUTE FUNCTION butler_memory_tool_payload_fence();
CREATE TRIGGER memory_approval_payload_fence BEFORE INSERT OR UPDATE ON approvals FOR EACH ROW EXECUTE FUNCTION butler_memory_tool_payload_fence();
CREATE TRIGGER memory_approval_event_payload_fence BEFORE INSERT OR UPDATE ON approval_events FOR EACH ROW EXECUTE FUNCTION butler_memory_tool_payload_fence();

-- A result arriving after deletion must not become another primary transcript
-- copy. These are the same existing current-run release guards, not new authority.
CREATE TRIGGER memory_tool_publication_fence BEFORE INSERT ON tool_calls FOR EACH ROW EXECUTE FUNCTION butler_memory_publication_fence();
CREATE TRIGGER summary_tool_publication_fence BEFORE INSERT ON tool_calls FOR EACH ROW EXECUTE FUNCTION butler_memory_summary_publication_fence();
CREATE TRIGGER temporal_tool_publication_fence BEFORE INSERT ON tool_calls FOR EACH ROW EXECUTE FUNCTION butler_memory_temporal_publication_fence();
CREATE TRIGGER experience_tool_publication_fence BEFORE INSERT ON tool_calls FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_publication_fence();

-- Earlier releases already advanced the external-anchor watermark. Reconcile
-- must not replay or renumber those deletions merely to erase this missed cache.
-- Backfill only runs with a currently deleted/revoked pin or redacted message,
-- using a real retained deletion of that same owner. No body is read into Go.
DO $$
DECLARE entry RECORD;
BEGIN
 FOR entry IN
  WITH affected AS (
   SELECT p.run_id FROM memory_run_sources p JOIN memory_sources s USING(source_id) WHERE s.status IN ('deleted','revoked')
   UNION SELECT run_id FROM messages WHERE privacy_redacted
  )
  SELECT DISTINCT ON(r.run_id) r.run_id,d.deletion_id FROM affected a JOIN runs r USING(run_id)
  JOIN sessions s USING(session_key) JOIN memory_deletions d ON d.owner_id=s.user_id
  ORDER BY r.run_id,d.watermark DESC,d.deletion_id
 LOOP
  PERFORM set_config('butler.memory_redaction',entry.deletion_id,TRUE);
  INSERT INTO memory_run_payload_redactions VALUES(entry.run_id,entry.deletion_id) ON CONFLICT DO NOTHING;
 END LOOP;
 UPDATE tool_calls t SET args='{}',result=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',d.redaction_id),error='{}'
 FROM memory_run_payload_redactions d WHERE t.run_id=d.run_id;
 UPDATE approvals a SET args_json='{}',summary='',details_json='{}',resolution_reason='privacy_redacted',status=CASE WHEN status='pending' THEN 'failed' ELSE status END,updated_at=clock_timestamp()
 FROM memory_run_payload_redactions d WHERE a.run_id=d.run_id;
 UPDATE approval_events e SET reason='privacy_redacted',metadata_json=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',d.redaction_id)
 FROM memory_run_payload_redactions d WHERE e.run_id=d.run_id;
END $$;

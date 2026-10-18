-- A stopped original has no fabricated assistant final/result. Only immutable,
-- content-free identities of the relinquished input/attempt are retained here.
CREATE FUNCTION butler_memory_experience_input_hash(i accepted_inputs) RETURNS TEXT IMMUTABLE STRICT LANGUAGE SQL AS $$
 SELECT encode(sha256(convert_to(jsonb_build_array(i.input_id,i.session_key,i.owner_id,i.task_id,i.run_id,i.status,i.attempts,i.checkpoint,i.checkpoint_json,i.error_code)::text,'UTF8')),'hex')
$$;
CREATE TABLE memory_experience_input_heads (
 input_id TEXT PRIMARY KEY,
 row_hash TEXT NOT NULL CHECK(length(row_hash)=64),
 revision BIGINT NOT NULL CHECK(revision>0),
 alive BOOLEAN NOT NULL
);
LOCK TABLE accepted_inputs IN SHARE ROW EXCLUSIVE MODE;
INSERT INTO memory_experience_input_heads(input_id,row_hash,revision,alive)
 SELECT i.input_id,butler_memory_experience_input_hash(i),1,TRUE FROM accepted_inputs i;
CREATE FUNCTION butler_memory_experience_input_head_update() RETURNS TRIGGER LANGUAGE plpgsql AS $$
 DECLARE digest TEXT; selected TEXT;
 BEGIN
  -- Readers hold only this shared per-input fence, never a primary input row
  -- lock. A writer may hold its primary row while waiting here without a lock
  -- inversion against a reader's source/tool locks. All payload access stays
  -- in the owning input service; the observer checks the small metadata head.
  FOR selected IN SELECT id FROM (SELECT CASE WHEN TG_OP='INSERT' THEN NEW.input_id ELSE OLD.input_id END AS id
    UNION SELECT CASE WHEN TG_OP='DELETE' THEN OLD.input_id ELSE NEW.input_id END) ids ORDER BY id
  LOOP
   PERFORM pg_advisory_xact_lock(hashtextextended('butler-terminal-input:'||selected,0));
  END LOOP;
  IF TG_OP='DELETE' OR (TG_OP='UPDATE' AND OLD.input_id IS DISTINCT FROM NEW.input_id) THEN
   UPDATE memory_experience_input_heads SET revision=revision+1,alive=FALSE WHERE input_id=OLD.input_id;
  END IF;
  IF TG_OP<>'DELETE' THEN
   -- Heartbeats/retention times do not revise the observed attempt payload.
   IF TG_OP='UPDATE' AND ROW(OLD.input_id,OLD.session_key,OLD.owner_id,OLD.task_id,OLD.run_id,OLD.status,OLD.attempts,OLD.checkpoint,OLD.checkpoint_json,OLD.error_code)
    IS NOT DISTINCT FROM ROW(NEW.input_id,NEW.session_key,NEW.owner_id,NEW.task_id,NEW.run_id,NEW.status,NEW.attempts,NEW.checkpoint,NEW.checkpoint_json,NEW.error_code) THEN RETURN NULL; END IF;
   digest:=butler_memory_experience_input_hash(NEW);
   INSERT INTO memory_experience_input_heads(input_id,row_hash,revision,alive)VALUES(NEW.input_id,digest,1,TRUE)
   ON CONFLICT(input_id)DO UPDATE SET row_hash=EXCLUDED.row_hash,revision=memory_experience_input_heads.revision+1,alive=TRUE;
  END IF;
  RETURN NULL;
 END
$$;
CREATE TRIGGER experience_input_head_update AFTER INSERT OR UPDATE OR DELETE ON accepted_inputs FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_input_head_update();

CREATE TABLE memory_experience_terminal_attempts (
 source_id TEXT PRIMARY KEY REFERENCES memory_experience_observations(source_id),
 input_id TEXT NOT NULL,
 input_status TEXT NOT NULL CHECK(input_status IN ('blocked','failed')),
 attempts INTEGER NOT NULL CHECK(attempts>0),
 checkpoint TEXT NOT NULL,
 input_hash TEXT NOT NULL CHECK(length(input_hash)=64),
 input_revision BIGINT NOT NULL CHECK(input_revision>0),
 reason TEXT NOT NULL CHECK(reason IN ('effect_unknown','execution_stopped','execution_deadline_exceeded','prior_attempt_terminal')),
 run_state TEXT NOT NULL CHECK(run_state IN ('failed','timed_out','tool_running'))
);
CREATE TRIGGER experience_terminal_attempts_immutable BEFORE UPDATE OR DELETE ON memory_experience_terminal_attempts FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();

CREATE FUNCTION butler_memory_terminal_run_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
 DECLARE selected TEXT;
 BEGIN
  FOR selected IN SELECT a.input_id FROM memory_experience_terminal_attempts a JOIN memory_experience_observations o USING(source_id) WHERE o.run_id=OLD.run_id ORDER BY a.input_id
  LOOP
   PERFORM pg_advisory_xact_lock(hashtextextended('butler-terminal-input:'||selected,0));
  END LOOP;
  RETURN NULL;
 END
$$;
CREATE TRIGGER experience_terminal_run_fence AFTER UPDATE OF run_id,session_key,task_id,task_revision,current_state OR DELETE ON runs FOR EACH ROW EXECUTE FUNCTION butler_memory_terminal_run_fence();

CREATE FUNCTION butler_lock_memory_terminal_attempts(ids TEXT[]) RETURNS VOID VOLATILE LANGUAGE plpgsql AS $$
 DECLARE selected TEXT;
 BEGIN
  FOR selected IN WITH RECURSIVE roots AS(SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id)
   SELECT DISTINCT a.input_id FROM memory_experience_terminal_attempts a JOIN roots r ON r.id=a.source_id ORDER BY a.input_id
  LOOP
   PERFORM pg_advisory_xact_lock_shared(hashtextextended('butler-terminal-input:'||selected,0));
  END LOOP;
 END
$$;

CREATE FUNCTION butler_memory_terminal_attempt_current(sid TEXT) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 SELECT EXISTS(SELECT 1 FROM memory_experience_terminal_attempts a
 JOIN memory_experience_observations o USING(source_id)
 JOIN accepted_inputs i ON i.input_id=a.input_id JOIN runs r ON r.run_id=o.run_id
 JOIN memory_experience_input_heads h ON h.input_id=i.input_id
 WHERE a.source_id=sid AND i.run_id=o.run_id AND i.task_id=o.task_id AND r.task_id=o.task_id
 AND i.owner_id=o.owner_id AND i.session_key=o.session_key AND r.session_key=o.session_key
 AND o.result_id='' AND i.status=a.input_status AND i.attempts=a.attempts
 AND i.checkpoint=a.checkpoint AND h.row_hash=a.input_hash AND h.revision=a.input_revision AND h.alive
 AND i.error_code=a.reason AND i.claim_id='' AND i.claim_expires_at IS NULL AND r.current_state=a.run_state
 AND (a.run_state IN ('failed','timed_out') OR (a.input_status='blocked' AND a.reason='effect_unknown' AND a.run_state='tool_running')))
$$;

CREATE OR REPLACE FUNCTION butler_memory_experience_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 WITH RECURSIVE roots AS(
  SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id
 ),observations AS(SELECT o.* FROM memory_experience_observations o JOIN roots r ON r.id=o.source_id)
 SELECT NOT EXISTS(SELECT 1 FROM observations o LEFT JOIN runs r ON r.run_id=o.run_id WHERE r.session_key IS DISTINCT FROM o.session_key)
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_tool_pins p ON p.observation_source_id=o.source_id
 LEFT JOIN memory_experience_tool_heads h ON h.tool_call_id=p.tool_call_id
 WHERE h.run_id IS DISTINCT FROM p.run_id OR p.run_id<>o.run_id OR h.row_hash IS DISTINCT FROM p.row_hash
 OR h.revision IS DISTINCT FROM p.row_revision OR h.alive IS DISTINCT FROM TRUE)
 AND NOT EXISTS(SELECT 1 FROM observations o WHERE NOT EXISTS(SELECT 1 FROM memory_experience_tool_pins p WHERE p.observation_source_id=o.source_id))
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_terminal_attempts a USING(source_id) WHERE NOT butler_memory_terminal_attempt_current(o.source_id))
$$;

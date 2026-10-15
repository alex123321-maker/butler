-- Experience bodies stay in the deletion-aware Catalog/Episodic Memory graph.
-- These tables contain identities/hashes/enums only, never recipe/response text.
CREATE TABLE memory_experience_observations (
 source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),
 source_revision BIGINT NOT NULL CHECK(source_revision>0),
 owner_id TEXT NOT NULL,
 observation_hash TEXT NOT NULL CHECK(length(observation_hash)=64),
 task_id TEXT NOT NULL,
 run_id TEXT NOT NULL,
 result_id TEXT NOT NULL,
 session_key TEXT NOT NULL,
 transcript_revision BIGINT NOT NULL CHECK(transcript_revision>0),
 UNIQUE(owner_id,run_id,result_id,observation_hash)
);
CREATE TABLE memory_experience_tool_pins (
 observation_source_id TEXT NOT NULL REFERENCES memory_experience_observations(source_id),
 tool_call_id TEXT NOT NULL,
 run_id TEXT NOT NULL,
 row_hash TEXT NOT NULL CHECK(length(row_hash)=64),
 row_revision BIGINT NOT NULL CHECK(row_revision>0),
 PRIMARY KEY(observation_source_id,tool_call_id)
);
CREATE INDEX memory_experience_tool_pin_call ON memory_experience_tool_pins(tool_call_id);

-- JSONB is the primary stored representation. Explicit epoch timestamps keep
-- the fingerprint independent of connection TimeZone/DateStyle. NULL differs
-- from an empty object. This function returns no response/argument bytes.
CREATE FUNCTION butler_memory_experience_tool_hash(t tool_calls) RETURNS TEXT IMMUTABLE STRICT LANGUAGE SQL AS $$
 SELECT encode(sha256(convert_to(jsonb_build_array(t.tool_call_id,t.run_id,t.tool_name,t.args,t.status,t.runtime_target,
 extract(epoch FROM t.started_at),extract(epoch FROM t.finished_at),t.result,t.error)::text,'UTF8')),'hex')
$$;
-- Transcript owns this current metadata projection. Keep tombstones so an
-- identical same-ID reinsert cannot revive a previously captured observation.
CREATE TABLE memory_experience_tool_heads (
 tool_call_id TEXT PRIMARY KEY,
 run_id TEXT NOT NULL,
 row_hash TEXT NOT NULL CHECK(length(row_hash)=64),
 revision BIGINT NOT NULL CHECK(revision>0),
 alive BOOLEAN NOT NULL
);
LOCK TABLE tool_calls IN SHARE ROW EXCLUSIVE MODE;
INSERT INTO memory_experience_tool_heads(tool_call_id,run_id,row_hash,revision,alive)
 SELECT t.tool_call_id,t.run_id,butler_memory_experience_tool_hash(t),1,TRUE FROM tool_calls t;
CREATE FUNCTION butler_memory_experience_tool_head_update() RETURNS TRIGGER LANGUAGE plpgsql AS $$
 BEGIN
  IF TG_OP='DELETE' OR (TG_OP='UPDATE' AND OLD.tool_call_id IS DISTINCT FROM NEW.tool_call_id) THEN
   UPDATE memory_experience_tool_heads SET revision=revision+1,alive=FALSE WHERE tool_call_id=OLD.tool_call_id;
  END IF;
  IF TG_OP<>'DELETE' THEN
   INSERT INTO memory_experience_tool_heads(tool_call_id,run_id,row_hash,revision,alive)
    VALUES(NEW.tool_call_id,NEW.run_id,butler_memory_experience_tool_hash(NEW),1,TRUE)
   ON CONFLICT(tool_call_id)DO UPDATE SET run_id=EXCLUDED.run_id,row_hash=EXCLUDED.row_hash,
    revision=memory_experience_tool_heads.revision+1,alive=TRUE;
  END IF;
  RETURN NULL;
 END
$$;
CREATE TRIGGER experience_tool_head_update AFTER INSERT OR UPDATE OR DELETE ON tool_calls FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_tool_head_update();
CREATE FUNCTION butler_memory_experience_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 WITH RECURSIVE roots AS(
  SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id
 ),observations AS(SELECT o.* FROM memory_experience_observations o JOIN roots r ON r.id=o.source_id)
 SELECT NOT EXISTS(SELECT 1 FROM observations o LEFT JOIN runs r ON r.run_id=o.run_id WHERE r.session_key IS DISTINCT FROM o.session_key)
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_tool_pins p ON p.observation_source_id=o.source_id
 LEFT JOIN memory_experience_tool_heads h ON h.tool_call_id=p.tool_call_id
 WHERE h.run_id IS DISTINCT FROM p.run_id OR p.run_id<>o.run_id OR h.row_hash IS DISTINCT FROM p.row_hash
 OR h.revision IS DISTINCT FROM p.row_revision OR h.alive IS DISTINCT FROM TRUE)
 AND NOT EXISTS(SELECT 1 FROM observations o WHERE NOT EXISTS(SELECT 1 FROM memory_experience_tool_pins p WHERE p.observation_source_id=o.source_id))
$$;

-- Add a current observation veto to every existing run copy/publication seam.
-- Complete source access and locked checks remain the owning service's duty.
CREATE FUNCTION butler_memory_experience_publication_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
 DECLARE rid TEXT;
 BEGIN
  IF TG_TABLE_NAME='messages' THEN IF NEW.role='user' THEN RETURN NEW; END IF; END IF;
  IF TG_TABLE_NAME='tasks' THEN rid:=NEW.last_run_id; ELSE rid:=NEW.run_id; END IF;
  IF NOT butler_memory_experience_sources_current(ARRAY(SELECT source_id FROM memory_run_sources WHERE run_id=rid)) THEN
   RAISE EXCEPTION 'run experience tool observations changed' USING ERRCODE='40001';
  END IF;
  RETURN NEW;
 END
$$;
CREATE TRIGGER experience_final_publication_fence BEFORE INSERT ON run_final_results FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_publication_fence();
CREATE TRIGGER experience_artifact_publication_fence BEFORE INSERT ON artifacts FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_publication_fence();
CREATE TRIGGER experience_message_publication_fence BEFORE INSERT ON messages FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_publication_fence();
CREATE TRIGGER experience_working_publication_fence BEFORE INSERT OR UPDATE ON memory_working FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_publication_fence();
CREATE TRIGGER experience_task_working_publication_fence BEFORE UPDATE OF working_json ON tasks FOR EACH ROW EXECUTE FUNCTION butler_memory_experience_publication_fence();
CREATE TABLE memory_experience_lessons (
 source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),
 source_revision BIGINT NOT NULL CHECK(source_revision>0),
 owner_id TEXT NOT NULL,
 claim_id TEXT NOT NULL UNIQUE REFERENCES memory_claims(claim_id),
 observation_source_id TEXT NOT NULL REFERENCES memory_experience_observations(source_id),
 lesson_hash TEXT NOT NULL CHECK(length(lesson_hash)=64),
 environment_hash TEXT NOT NULL CHECK(length(environment_hash)=64),
 dependencies_json JSONB NOT NULL CHECK(jsonb_typeof(dependencies_json)='array'),
 task_id TEXT NOT NULL, run_id TEXT NOT NULL, result_id TEXT NOT NULL,
 verification TEXT NOT NULL CHECK(verification IN ('owner_negative','task_unverified','outcome_unknown','owner_verified_with_observed_actions'))
);
CREATE TABLE memory_experience_feedback (
 feedback_id TEXT PRIMARY KEY,
 owner_id TEXT NOT NULL,
 source_id TEXT NOT NULL REFERENCES memory_experience_lessons(source_id),
 run_id TEXT NOT NULL,
 verdict TEXT NOT NULL CHECK(verdict IN ('useful','misapplied','correction_required','negative','unknown')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(owner_id,source_id,run_id)
);
CREATE TABLE memory_experience_pins (
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 source_id TEXT NOT NULL REFERENCES memory_experience_lessons(source_id),
 environment_hash TEXT NOT NULL CHECK(length(environment_hash)=64),
 lesson_hash TEXT NOT NULL CHECK(length(lesson_hash)=64),
 slot SMALLINT NOT NULL CHECK(slot BETWEEN 1 AND 8),
 PRIMARY KEY(run_id,source_id),
 UNIQUE(run_id,slot)
);
-- Applicability follows every exact lesson ancestor, including a consolidated
-- presentation or reviewed skill. This adds no source access or tool authority.
CREATE FUNCTION butler_memory_experience_lessons_eligible(ids TEXT[],environment TEXT) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 WITH RECURSIVE roots AS(SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id),
 lessons AS(SELECT l.* FROM memory_experience_lessons l JOIN roots r ON r.id=l.source_id LIMIT 9)
 SELECT (SELECT count(*) FROM lessons)<=8 AND NOT EXISTS(
  SELECT 1 FROM lessons l LEFT JOIN memory_sources s USING(source_id) LEFT JOIN memory_claims c ON c.claim_id=l.claim_id
  WHERE environment IS NULL OR environment='' OR l.environment_hash<>environment OR s.head_revision IS DISTINCT FROM l.source_revision
  OR s.status IS DISTINCT FROM 'active' OR s.owner_id IS DISTINCT FROM l.owner_id OR c.status IS DISTINCT FROM 'confirmed'
  OR c.source_id IS DISTINCT FROM l.source_id OR c.source_revision IS DISTINCT FROM l.source_revision
  OR EXISTS(SELECT 1 FROM memory_experience_feedback f WHERE f.source_id=l.source_id AND f.verdict IN('negative','misapplied','correction_required')))
$$;
CREATE FUNCTION butler_experience_metadata_immutable() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'immutable episodic experience provenance'; END $$;
CREATE TRIGGER experience_observations_immutable BEFORE UPDATE OR DELETE ON memory_experience_observations FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();
CREATE TRIGGER experience_tool_pins_immutable BEFORE UPDATE OR DELETE ON memory_experience_tool_pins FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();
CREATE TRIGGER experience_lessons_immutable BEFORE UPDATE OR DELETE ON memory_experience_lessons FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();
CREATE TRIGGER experience_feedback_immutable BEFORE UPDATE OR DELETE ON memory_experience_feedback FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();
CREATE TRIGGER experience_pins_immutable BEFORE UPDATE OR DELETE ON memory_experience_pins FOR EACH ROW EXECUTE FUNCTION butler_experience_metadata_immutable();

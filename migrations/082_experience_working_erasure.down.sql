DO $$ BEGIN
 -- A failed first deletion can advance the external anchor while its database
 -- transaction rolls back. SQL deletion counts cannot prove no pending replay.
 IF EXISTS(SELECT 1 FROM memory_experience_observations) THEN
  RAISE EXCEPTION 'retain experience erasure for populated provenance';
 END IF;
END $$;
CREATE OR REPLACE FUNCTION butler_memory_experience_publication_fence() RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE rid TEXT;
BEGIN
 IF TG_TABLE_NAME='messages' THEN IF NEW.role='user' THEN RETURN NEW; END IF; END IF;
 IF TG_TABLE_NAME='tasks' THEN rid:=NEW.last_run_id; ELSE rid:=NEW.run_id; END IF;
 IF NOT butler_memory_experience_sources_current(ARRAY(SELECT source_id FROM memory_run_sources WHERE run_id=rid)) THEN
  RAISE EXCEPTION 'run experience tool observations changed' USING ERRCODE='40001';
 END IF;
 RETURN NEW;
END $$;

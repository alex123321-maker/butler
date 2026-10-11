-- Kernel continuation identities contain references and hashes, never source bodies.
CREATE TABLE converter_parent_continuations (
 continuation_id TEXT PRIMARY KEY,
 child_task_id TEXT NOT NULL REFERENCES converter_developer_snapshots(task_id),
 owner_id TEXT NOT NULL,
 parent_task_id TEXT NOT NULL REFERENCES tasks(task_id),
 parent_revision BIGINT NOT NULL CHECK(parent_revision>0),
 constraints_revision BIGINT NOT NULL CHECK(constraints_revision>0),
 purpose TEXT NOT NULL CHECK(purpose IN ('candidate_return','activated_catalog')),
 intent_hash TEXT NOT NULL CHECK(length(intent_hash)=64),
 intent_json JSONB NOT NULL,
 deadline TIMESTAMPTZ NOT NULL,
 state TEXT NOT NULL DEFAULT 'waiting' CHECK(state IN ('waiting','queued')),
 input_id TEXT UNIQUE REFERENCES accepted_inputs(input_id),
 ready_hash TEXT NOT NULL DEFAULT '',
 ready_json JSONB,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(child_task_id,purpose),
 CHECK((state='waiting' AND input_id IS NULL AND ready_json IS NULL AND ready_hash='') OR (state='queued' AND input_id IS NOT NULL AND ready_json IS NOT NULL AND length(ready_hash)=64))
);
CREATE FUNCTION converter_continuation_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'converter continuation history is immutable'; END IF;
 IF OLD.state<>'waiting' OR NEW.state<>'queued' OR
    (to_jsonb(NEW)-ARRAY['state','input_id','ready_hash','ready_json']) IS DISTINCT FROM
    (to_jsonb(OLD)-ARRAY['state','input_id','ready_hash','ready_json']) THEN
  RAISE EXCEPTION 'converter continuation identity is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER converter_continuation_guard BEFORE UPDATE OR DELETE ON converter_parent_continuations
 FOR EACH ROW EXECUTE FUNCTION converter_continuation_immutable();

-- Dormant kernel foundation. No channel adapter or admission is enabled here.
CREATE TABLE communication_bindings (
 binding_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, channel TEXT NOT NULL CHECK(channel IN ('web','telegram')),
 recipient TEXT NOT NULL, version BIGINT NOT NULL CHECK(version>0), enabled BOOLEAN NOT NULL,
 verified BOOLEAN NOT NULL CHECK(verified), capabilities JSONB NOT NULL CHECK(jsonb_typeof(capabilities)='array'),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE communication_admissions (
 input_id TEXT PRIMARY KEY REFERENCES accepted_inputs(input_id) ON DELETE CASCADE,
 owner_id TEXT NOT NULL, session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
 schema_epoch INT NOT NULL CHECK(schema_epoch=1), methods JSONB NOT NULL CHECK(jsonb_typeof(methods)='array'),
 quiet BOOLEAN NOT NULL DEFAULT FALSE, obligation_state TEXT NOT NULL CHECK(obligation_state IN ('open','waiting_for_user','response_committed','closed_with_failure','cancelled')),
 correction_count INT NOT NULL DEFAULT 0 CHECK(correction_count BETWEEN 0 AND 1), publication_count INT NOT NULL DEFAULT 0 CHECK(publication_count BETWEEN 0 AND 20),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE communication_publications (
 publication_id TEXT PRIMARY KEY, effect_id TEXT NOT NULL UNIQUE, tool_call_id TEXT NOT NULL,
 input_id TEXT NOT NULL REFERENCES communication_admissions(input_id),
 owner_id TEXT NOT NULL, session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
 run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE, task_id TEXT REFERENCES tasks(task_id) ON DELETE CASCADE,
 task_revision BIGINT NOT NULL, constraints_revision BIGINT NOT NULL,
 kind TEXT NOT NULL, purpose TEXT NOT NULL, payload JSONB NOT NULL, payload_hash TEXT NOT NULL,
 source_refs JSONB NOT NULL, receipt_json JSONB NOT NULL,
 publication_sequence BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION butler_communication_claim_epoch() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.status='running' AND NEW.claim_id IS DISTINCT FROM OLD.claim_id AND
    EXISTS(SELECT 1 FROM communication_admissions WHERE input_id=NEW.input_id AND schema_epoch>COALESCE(NULLIF(current_setting('butler.communication_epoch',TRUE),''),'0')::int)
 THEN RAISE EXCEPTION 'unsupported communication admission epoch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER communication_claim_epoch BEFORE UPDATE OF status,claim_id ON accepted_inputs FOR EACH ROW EXECUTE FUNCTION butler_communication_claim_epoch();
CREATE TABLE communication_interactions (
 interaction_id TEXT PRIMARY KEY, publication_id TEXT NOT NULL UNIQUE REFERENCES communication_publications(publication_id) ON DELETE CASCADE,
 input_id TEXT NOT NULL REFERENCES accepted_inputs(input_id), owner_id TEXT NOT NULL,
 session_key TEXT NOT NULL, run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
 task_id TEXT REFERENCES tasks(task_id) ON DELETE CASCADE, task_revision BIGINT NOT NULL, constraints_revision BIGINT NOT NULL,
 question_json JSONB NOT NULL, state TEXT NOT NULL CHECK(state IN ('open','resolved','cancelled','expired','superseded')),
 version BIGINT NOT NULL DEFAULT 1, expires_at TIMESTAMPTZ, response_json JSONB, response_hash TEXT,
 response_delivery_id TEXT, response_binding_id TEXT, resolved_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX communication_one_blocking_question ON communication_interactions(input_id) WHERE state='open';
CREATE TABLE communication_events (
 event_id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, owner_id TEXT NOT NULL,
 session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
 aggregate_id TEXT NOT NULL, event_type TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
ALTER TABLE delivery_outbox ALTER COLUMN result_id DROP NOT NULL;
ALTER TABLE delivery_outbox ADD COLUMN publication_id TEXT REFERENCES communication_publications(publication_id) ON DELETE CASCADE;
ALTER TABLE delivery_outbox ADD COLUMN binding_id TEXT;
ALTER TABLE delivery_outbox ADD COLUMN binding_version BIGINT;
ALTER TABLE delivery_outbox ADD COLUMN recipient_snapshot TEXT;
ALTER TABLE delivery_outbox ADD COLUMN representation_json JSONB;
ALTER TABLE delivery_outbox ADD CONSTRAINT delivery_payload_owner CHECK((result_id IS NOT NULL AND publication_id IS NULL) OR (result_id IS NULL AND publication_id IS NOT NULL AND binding_id IS NOT NULL AND binding_version>0 AND recipient_snapshot IS NOT NULL AND representation_json IS NOT NULL));
CREATE UNIQUE INDEX communication_delivery_identity ON delivery_outbox(publication_id,binding_id,binding_version,chunk_index) WHERE publication_id IS NOT NULL;
-- Existing channel workers explicitly exclude publication rows. New rows are
-- claimed only by an explicitly constructed communication ClaimStore.
CREATE FUNCTION butler_immutable_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'communication publication is immutable'; END $$;
CREATE TRIGGER immutable_communication_publication BEFORE UPDATE ON communication_publications FOR EACH ROW EXECUTE FUNCTION butler_immutable_publication();
CREATE FUNCTION butler_immutable_publication_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.publication_id IS DISTINCT FROM OLD.publication_id OR NEW.binding_id IS DISTINCT FROM OLD.binding_id OR NEW.binding_version IS DISTINCT FROM OLD.binding_version OR NEW.recipient_snapshot IS DISTINCT FROM OLD.recipient_snapshot OR NEW.representation_json IS DISTINCT FROM OLD.representation_json THEN RAISE EXCEPTION 'publication delivery identity is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_publication_delivery BEFORE UPDATE ON delivery_outbox FOR EACH ROW EXECUTE FUNCTION butler_immutable_publication_delivery();

CREATE TABLE run_final_results (
 result_id TEXT PRIMARY KEY, run_id TEXT NOT NULL UNIQUE REFERENCES runs(run_id) ON DELETE CASCADE,
 session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
 task_id TEXT REFERENCES tasks(task_id) ON DELETE CASCADE, task_revision BIGINT NOT NULL DEFAULT 0,
 constraints_revision BIGINT NOT NULL DEFAULT 0, artifact_id TEXT NOT NULL REFERENCES artifacts(artifact_id),
 message_id TEXT NOT NULL REFERENCES messages(message_id), content TEXT NOT NULL, content_hash TEXT NOT NULL,
 answer_state TEXT NOT NULL CHECK(answer_state IN ('final','partial')),
 verdict TEXT NOT NULL DEFAULT 'unverified' CHECK(verdict='unverified'),
 effects_json JSONB NOT NULL DEFAULT '[]', limitations_json JSONB NOT NULL DEFAULT '[]',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE FUNCTION butler_immutable_final_result() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'final result is immutable'; END $$;
CREATE TRIGGER immutable_final_result BEFORE UPDATE ON run_final_results FOR EACH ROW EXECUTE FUNCTION butler_immutable_final_result();
CREATE FUNCTION butler_protect_final_components() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='artifacts' THEN
  IF EXISTS(SELECT 1 FROM run_final_results WHERE artifact_id=OLD.artifact_id) THEN RAISE EXCEPTION 'final artifact is immutable'; END IF;
 ELSE
  IF EXISTS(SELECT 1 FROM run_final_results WHERE message_id=OLD.message_id) THEN RAISE EXCEPTION 'final transcript is immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_final_artifact BEFORE UPDATE ON artifacts FOR EACH ROW EXECUTE FUNCTION butler_protect_final_components();
CREATE TRIGGER immutable_final_message BEFORE UPDATE ON messages FOR EACH ROW EXECUTE FUNCTION butler_protect_final_components();
CREATE TABLE delivery_outbox (
 notification_id TEXT PRIMARY KEY, result_id TEXT NOT NULL REFERENCES run_final_results(result_id) ON DELETE CASCADE,
 run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE, session_key TEXT NOT NULL,
 channel TEXT NOT NULL, chunk_index INT NOT NULL CHECK(chunk_index>=0), content TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','sending','sent','result_ready','delivery_unknown','failed')),
 attempt_count INT NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 deadline_at TIMESTAMPTZ NOT NULL, claim_id TEXT NOT NULL DEFAULT '', claim_until TIMESTAMPTZ,
 channel_ack TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(result_id,channel,chunk_index)
);
CREATE INDEX delivery_outbox_ready ON delivery_outbox(next_attempt_at) WHERE status='pending';
CREATE FUNCTION butler_immutable_notification() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.notification_id IS DISTINCT FROM OLD.notification_id OR NEW.result_id IS DISTINCT FROM OLD.result_id OR
    NEW.run_id IS DISTINCT FROM OLD.run_id OR NEW.session_key IS DISTINCT FROM OLD.session_key OR
    NEW.channel IS DISTINCT FROM OLD.channel OR NEW.chunk_index IS DISTINCT FROM OLD.chunk_index OR
    NEW.content IS DISTINCT FROM OLD.content OR NEW.deadline_at IS DISTINCT FROM OLD.deadline_at THEN
  RAISE EXCEPTION 'notification payload is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_notification BEFORE UPDATE ON delivery_outbox FOR EACH ROW EXECUTE FUNCTION butler_immutable_notification();
CREATE TABLE completion_aux_jobs (
 job_id TEXT PRIMARY KEY, result_id TEXT NOT NULL REFERENCES run_final_results(result_id) ON DELETE CASCADE,
 run_id TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE, session_key TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('working_cleanup','transient_cleanup','memory_pipeline')),
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','done','failed')),
 attempt_count INT NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 claim_id TEXT NOT NULL DEFAULT '', claim_until TIMESTAMPTZ, error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

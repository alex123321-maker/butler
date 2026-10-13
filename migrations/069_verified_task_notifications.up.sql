ALTER TABLE tasks ADD COLUMN result_delivery TEXT NOT NULL DEFAULT 'attempt'
 CHECK(result_delivery IN ('attempt','verified_task'));
CREATE TABLE task_verified_notifications (
 notification_id TEXT PRIMARY KEY, task_id TEXT NOT NULL UNIQUE REFERENCES tasks(task_id),
 result_id TEXT NOT NULL, task_revision BIGINT NOT NULL, constraints_revision BIGINT NOT NULL,
 owner_id TEXT NOT NULL, run_id TEXT NOT NULL REFERENCES runs(run_id),
 session_key TEXT NOT NULL REFERENCES sessions(session_key),
 channel TEXT NOT NULL CHECK(channel IN ('web','telegram')), content TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','sending','sent','result_ready','delivery_unknown','failed')),
 attempt_count INT NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 deadline_at TIMESTAMPTZ NOT NULL, claim_id TEXT NOT NULL DEFAULT '', claim_until TIMESTAMPTZ,
 channel_ack TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(task_id,result_id) REFERENCES task_results(task_id,result_id)
);
CREATE INDEX task_verified_notification_ready ON task_verified_notifications(next_attempt_at) WHERE status='pending';
CREATE FUNCTION butler_immutable_task_result_delivery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.result_delivery IS DISTINCT FROM OLD.result_delivery THEN
  RAISE EXCEPTION 'task result delivery is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_task_result_delivery BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION butler_immutable_task_result_delivery();
CREATE FUNCTION butler_immutable_verified_task_notification() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.notification_id,NEW.task_id,NEW.result_id,NEW.task_revision,NEW.constraints_revision,NEW.owner_id,NEW.run_id,NEW.session_key,NEW.channel,NEW.content,NEW.deadline_at)
 IS DISTINCT FROM (OLD.notification_id,OLD.task_id,OLD.result_id,OLD.task_revision,OLD.constraints_revision,OLD.owner_id,OLD.run_id,OLD.session_key,OLD.channel,OLD.content,OLD.deadline_at)
 THEN RAISE EXCEPTION 'verified task notification is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_verified_task_notification BEFORE UPDATE ON task_verified_notifications FOR EACH ROW EXECUTE FUNCTION butler_immutable_verified_task_notification();

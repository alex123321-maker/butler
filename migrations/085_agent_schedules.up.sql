CREATE TABLE scheduled_jobs (
 schedule_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL,
 session_key TEXT NOT NULL REFERENCES sessions(session_key),
 source_run_id TEXT NOT NULL REFERENCES runs(run_id),
 source_task_id TEXT REFERENCES tasks(task_id),
 source_constraints JSONB NOT NULL, source_goal TEXT NOT NULL,
 execution_policy TEXT NOT NULL CHECK(execution_policy IN ('normal','prepare_only')),
 authority_json JSONB, approved BOOLEAN NOT NULL,
 spec_json JSONB NOT NULL, revision BIGINT NOT NULL DEFAULT 1,
 state TEXT NOT NULL CHECK(state IN ('active','paused','blocked','cancelled','finished')),
 reason TEXT NOT NULL DEFAULT '', next_at TIMESTAMPTZ,
 expires_at TIMESTAMPTZ NOT NULL, maximum_occurrences INT NOT NULL CHECK(maximum_occurrences BETWEEN 1 AND 100),
 occurrence_count INT NOT NULL DEFAULT 0 CHECK(occurrence_count BETWEEN 0 AND 100),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX scheduled_jobs_due ON scheduled_jobs(next_at) WHERE state='active';
CREATE INDEX scheduled_jobs_owner ON scheduled_jobs(owner_id,session_key,schedule_id);
CREATE TABLE scheduled_occurrences (
 occurrence_id TEXT PRIMARY KEY, schedule_id TEXT NOT NULL REFERENCES scheduled_jobs(schedule_id),
 schedule_revision BIGINT NOT NULL, slot_key TEXT NOT NULL, scheduled_at TIMESTAMPTZ NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('admitted','notified','skipped','blocked')),
 reason TEXT NOT NULL, missed_count BIGINT NOT NULL DEFAULT 0,
 task_id TEXT UNIQUE REFERENCES tasks(task_id), input_id TEXT UNIQUE REFERENCES accepted_inputs(input_id),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), UNIQUE(schedule_id,schedule_revision,slot_key)
);
CREATE TABLE schedule_control_receipts (
 effect_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, session_key TEXT NOT NULL,
 request_hash TEXT NOT NULL, receipt_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION butler_schedule_receipt_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'schedule receipt is immutable'; END $$;
CREATE TRIGGER schedule_receipt_immutable BEFORE UPDATE OR DELETE ON schedule_control_receipts FOR EACH ROW EXECUTE FUNCTION butler_schedule_receipt_immutable();
CREATE TABLE schedule_reminder_outbox (
 notification_id TEXT PRIMARY KEY, occurrence_id TEXT NOT NULL REFERENCES scheduled_occurrences(occurrence_id),
 schedule_id TEXT NOT NULL REFERENCES scheduled_jobs(schedule_id), schedule_revision BIGINT NOT NULL,
 owner_id TEXT NOT NULL, session_key TEXT NOT NULL REFERENCES sessions(session_key),
 channel TEXT NOT NULL CHECK(channel IN ('web','telegram')), chunk_index INT NOT NULL, content TEXT NOT NULL,
 status TEXT NOT NULL CHECK(status IN ('pending','sending','sent','result_ready','delivery_unknown','failed')),
 attempt_count INT NOT NULL DEFAULT 0, next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 deadline_at TIMESTAMPTZ NOT NULL, claim_id TEXT NOT NULL DEFAULT '', claim_until TIMESTAMPTZ,
 channel_ack TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(occurrence_id,chunk_index)
);
CREATE INDEX schedule_reminder_ready ON schedule_reminder_outbox(next_attempt_at) WHERE status='pending';
CREATE FUNCTION butler_schedule_notice_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.notification_id,NEW.occurrence_id,NEW.schedule_id,NEW.schedule_revision,NEW.owner_id,NEW.session_key,NEW.channel,NEW.chunk_index,NEW.content,NEW.deadline_at)
 IS DISTINCT FROM (OLD.notification_id,OLD.occurrence_id,OLD.schedule_id,OLD.schedule_revision,OLD.owner_id,OLD.session_key,OLD.channel,OLD.chunk_index,OLD.content,OLD.deadline_at)
 THEN RAISE EXCEPTION 'schedule notice is immutable'; END IF; RETURN NEW;
END $$;
CREATE TRIGGER schedule_notice_immutable BEFORE UPDATE ON schedule_reminder_outbox FOR EACH ROW EXECUTE FUNCTION butler_schedule_notice_immutable();

-- A schedule supplies no tool authority. This additional fence prevents a queued
-- or running occurrence from publishing after cancellation, owner amendment or
-- source control. The ordinary mandate/source/budget checks remain independent.
CREATE FUNCTION butler_task_check_schedule(id TEXT) RETURNS void LANGUAGE plpgsql AS $$
DECLARE q scheduled_jobs%ROWTYPE; t tasks%ROWTYPE; o scheduled_occurrences%ROWTYPE;
BEGIN
 WITH RECURSIVE lineage AS (
  SELECT task_id,parent_task_id FROM tasks WHERE task_id=id
  UNION SELECT ancestor.task_id,ancestor.parent_task_id FROM tasks ancestor JOIN lineage p ON ancestor.task_id=p.parent_task_id
 ) SELECT occurrence.* INTO o FROM scheduled_occurrences occurrence JOIN lineage l ON l.task_id=occurrence.task_id LIMIT 1;
 IF NOT FOUND THEN RETURN; END IF;
 SELECT * INTO q FROM scheduled_jobs WHERE schedule_id=o.schedule_id FOR SHARE;
 IF NOT FOUND OR q.revision<>o.schedule_revision OR q.state IN ('paused','blocked','cancelled') OR q.expires_at<=clock_timestamp() THEN
  RAISE EXCEPTION 'scheduled task authority changed';
 END IF;
 IF q.source_task_id IS NOT NULL THEN
  SELECT * INTO t FROM tasks WHERE task_id=q.source_task_id FOR SHARE;
  IF NOT FOUND OR t.owner_id<>q.owner_id OR t.session_key<>q.session_key OR t.status IN ('paused','waiting_for_user','cancelled')
   OR t.constraints_json<>q.source_constraints OR t.execution_policy<>q.execution_policy OR t.goal<>q.source_goal
   OR COALESCE((t.working_json #>> '{scratch,privacy_redacted}')::boolean,FALSE) THEN
   RAISE EXCEPTION 'scheduled task source changed';
  END IF;
 END IF;
 PERFORM butler_memory_check_run_sources(q.source_run_id);
END $$;

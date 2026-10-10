ALTER TABLE execution_budget_scopes DROP CONSTRAINT execution_budget_scopes_kind_check;
ALTER TABLE execution_budget_scopes ADD CONSTRAINT execution_budget_scopes_kind_check CHECK(kind IN ('task','mandate','owner_day','conversation','auxiliary'));
CREATE TABLE memory_aux_jobs (
 job_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('index','extract','summary','ingest')),
 purpose TEXT NOT NULL, task_id TEXT NOT NULL DEFAULT '', task_revision BIGINT NOT NULL DEFAULT 0,
 constraints_revision BIGINT NOT NULL DEFAULT 0, original_run_id TEXT NOT NULL DEFAULT '',
 authority JSONB NOT NULL DEFAULT '{}', sources JSONB NOT NULL, source_fingerprint TEXT NOT NULL,
 allowance JSONB NOT NULL, deadline TIMESTAMPTZ NOT NULL,
 status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','running','done','failed','cancelled')),
 claim_id TEXT NOT NULL DEFAULT '', claim_epoch BIGINT NOT NULL DEFAULT 0, claim_until TIMESTAMPTZ,
 attempts INT NOT NULL DEFAULT 0, max_attempts INT NOT NULL DEFAULT 4 CHECK(max_attempts BETWEEN 1 AND 8),
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), error_code TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX memory_aux_ready ON memory_aux_jobs(next_attempt_at) WHERE status='pending';
CREATE FUNCTION butler_memory_aux_identity() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.job_id,NEW.owner_id,NEW.kind,NEW.purpose,NEW.task_id,NEW.task_revision,NEW.constraints_revision,NEW.original_run_id,NEW.authority,NEW.sources,NEW.source_fingerprint,NEW.allowance,NEW.deadline,NEW.max_attempts)
 IS DISTINCT FROM (OLD.job_id,OLD.owner_id,OLD.kind,OLD.purpose,OLD.task_id,OLD.task_revision,OLD.constraints_revision,OLD.original_run_id,OLD.authority,OLD.sources,OLD.source_fingerprint,OLD.allowance,OLD.deadline,OLD.max_attempts) THEN
  RAISE EXCEPTION 'auxiliary job authority is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER memory_aux_identity BEFORE UPDATE ON memory_aux_jobs FOR EACH ROW EXECUTE FUNCTION butler_memory_aux_identity();

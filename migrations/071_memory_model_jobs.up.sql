-- Extend the existing auxiliary queue and execution budget ledger.
CREATE TABLE memory_model_policies (
 policy_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, policy JSONB NOT NULL,
 state TEXT NOT NULL DEFAULT 'active' CHECK(state IN ('active','paused','revoked')),
 jobs_created INT NOT NULL DEFAULT 0, last_event TEXT NOT NULL DEFAULT '',
 next_event_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 reason TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION butler_memory_model_policy_identity() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF (NEW.policy_id,NEW.owner_id,NEW.policy,NEW.created_at) IS DISTINCT FROM (OLD.policy_id,OLD.owner_id,OLD.policy,OLD.created_at) OR OLD.state='revoked' AND NEW.state<>'revoked' THEN
  RAISE EXCEPTION 'background memory policy is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER memory_model_policy_identity BEFORE UPDATE ON memory_model_policies FOR EACH ROW EXECUTE FUNCTION butler_memory_model_policy_identity();
ALTER TABLE memory_aux_jobs DROP CONSTRAINT memory_aux_jobs_kind_check;
ALTER TABLE memory_aux_jobs ADD CONSTRAINT memory_aux_jobs_kind_check CHECK(kind IN ('index','extract','summary','ingest','model'));
ALTER TABLE memory_aux_jobs ADD COLUMN policy_id TEXT REFERENCES memory_model_policies(policy_id);
CREATE FUNCTION butler_memory_model_job_identity() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.policy_id IS DISTINCT FROM OLD.policy_id THEN RAISE EXCEPTION 'background policy binding is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER memory_model_job_identity BEFORE UPDATE ON memory_aux_jobs FOR EACH ROW EXECUTE FUNCTION butler_memory_model_job_identity();
CREATE TABLE memory_model_attempts (
 operation_id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES memory_aux_jobs(job_id),
 claim_id TEXT NOT NULL, claim_epoch BIGINT NOT NULL, reservation_id TEXT NOT NULL REFERENCES execution_budget_reservations(reservation_id),
 provider TEXT NOT NULL, model TEXT NOT NULL, source_fingerprint TEXT NOT NULL,
 locality TEXT NOT NULL CHECK(locality IN ('cloud','local')), quote JSONB NOT NULL,
 state TEXT NOT NULL DEFAULT 'reserved' CHECK(state IN ('reserved','dispatched','observed','published','failed')),
 usage JSONB, output_hash TEXT, result_source_id TEXT REFERENCES memory_sources(source_id),
 reason TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), observed_at TIMESTAMPTZ,
 UNIQUE(job_id,claim_epoch)
);
CREATE UNIQUE INDEX memory_model_one_result ON memory_model_attempts(job_id) WHERE state='published';
CREATE TABLE memory_model_control (
 singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK(singleton),
 state TEXT NOT NULL CHECK(state IN ('enabled','paused','disabled'))
);
INSERT INTO memory_model_control VALUES(TRUE,'disabled');

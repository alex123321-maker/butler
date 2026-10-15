DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_model_policies) OR EXISTS(SELECT 1 FROM memory_model_attempts) OR EXISTS(SELECT 1 FROM memory_aux_jobs WHERE kind='model') THEN
  RAISE EXCEPTION 'retain background policy, attempt and budget history; disable workers and roll application forward';
 END IF;
END $$;
DROP TABLE memory_model_control;
DROP TABLE memory_model_attempts;
DROP TRIGGER memory_model_job_identity ON memory_aux_jobs;
DROP FUNCTION butler_memory_model_job_identity();
ALTER TABLE memory_aux_jobs DROP COLUMN policy_id;
ALTER TABLE memory_aux_jobs DROP CONSTRAINT memory_aux_jobs_kind_check;
ALTER TABLE memory_aux_jobs ADD CONSTRAINT memory_aux_jobs_kind_check CHECK(kind IN ('index','extract','summary','ingest'));
DROP TABLE memory_model_policies;
DROP FUNCTION butler_memory_model_policy_identity();

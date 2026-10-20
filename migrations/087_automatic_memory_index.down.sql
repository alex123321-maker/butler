DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_index_policies) OR EXISTS(SELECT 1 FROM memory_index_policy_jobs) THEN
  RAISE EXCEPTION 'retain automatic index policy and job history; disable policies and roll application forward';
 END IF;
END $$;
DROP TABLE memory_index_policy_jobs;
DROP TABLE memory_index_policies;

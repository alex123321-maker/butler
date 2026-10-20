-- Explicit bounded owner permission for local maintenance, independent of runs.
CREATE TABLE memory_index_policies (
 policy_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, space_id TEXT NOT NULL,
 enabled BOOLEAN NOT NULL DEFAULT true, revision BIGINT NOT NULL DEFAULT 1,
 max_chunks INTEGER NOT NULL CHECK(max_chunks BETWEEN 1 AND 10000),
 max_jobs_per_day INTEGER NOT NULL CHECK(max_jobs_per_day BETWEEN 1 AND 96),
 allowance JSONB NOT NULL, expires_at TIMESTAMPTZ NOT NULL, quality_proof JSONB NOT NULL,
 last_fingerprint TEXT NOT NULL DEFAULT '', last_job_id TEXT NOT NULL DEFAULT '',
 day DATE NOT NULL DEFAULT CURRENT_DATE, jobs_today INTEGER NOT NULL DEFAULT 0,
 next_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX memory_one_enabled_index_policy ON memory_index_policies(owner_id) WHERE enabled;
CREATE TABLE memory_index_policy_jobs (
 job_id TEXT PRIMARY KEY REFERENCES memory_aux_jobs(job_id),
 policy_id TEXT NOT NULL REFERENCES memory_index_policies(policy_id),
 policy_revision BIGINT NOT NULL, expected_previous TEXT NOT NULL
);

-- Exact typed owner contexts are immutable job manifest data, never model input.
-- Nil legacy policy fields remain omitted; [] preserves existing local job rows.
ALTER TABLE memory_aux_jobs ADD COLUMN context_manifest JSONB NOT NULL DEFAULT '[]';
ALTER TABLE memory_aux_jobs ADD CONSTRAINT memory_job_context_manifest CHECK (
 jsonb_typeof(context_manifest)='array' AND jsonb_array_length(context_manifest)<=8
 AND (context_manifest='[]'::jsonb OR kind='model' AND purpose='memory-consolidation-v1' AND task_id='' AND original_run_id='')
);

-- Text remains exclusively in catalog revisions. These are immutable manifests,
-- current presentation heads and content-free owner decision/spend receipts.
CREATE TABLE memory_consolidation_inputs (
 source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),
 owner_id TEXT NOT NULL, view_key TEXT NOT NULL CHECK(length(view_key) BETWEEN 1 AND 128),
 policy_id TEXT NOT NULL UNIQUE REFERENCES memory_model_policies(policy_id) DEFERRABLE INITIALLY DEFERRED,
 request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
 source_units JSONB NOT NULL, claim_ids JSONB NOT NULL, temporal_query JSONB NOT NULL,
 retired BOOLEAN NOT NULL DEFAULT FALSE,
 cleanup_complete BOOLEAN NOT NULL DEFAULT FALSE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX memory_consolidation_input_owner ON memory_consolidation_inputs(owner_id,view_key,created_at);
CREATE TABLE memory_consolidation_generations (
 source_id TEXT PRIMARY KEY REFERENCES memory_sources(source_id),
 input_source_id TEXT NOT NULL UNIQUE REFERENCES memory_consolidation_inputs(source_id),
 owner_id TEXT NOT NULL, view_key TEXT NOT NULL,
 algorithm TEXT NOT NULL CHECK(algorithm='memory-consolidation-v1'),
 operation_id TEXT NOT NULL UNIQUE REFERENCES memory_model_attempts(operation_id),
 source_revision BIGINT NOT NULL CHECK(source_revision=1), content_hash TEXT NOT NULL CHECK(length(content_hash)=64),
 parent_manifest JSONB NOT NULL, unit_ids JSONB NOT NULL,
 base_source_id TEXT NOT NULL DEFAULT '', base_revision BIGINT NOT NULL,
 diff JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE memory_consolidation_heads (
 owner_id TEXT NOT NULL, view_key TEXT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 0,
 active_source_id TEXT REFERENCES memory_consolidation_generations(source_id),
 PRIMARY KEY(owner_id,view_key)
);
CREATE TABLE memory_consolidation_decisions (
 owner_id TEXT NOT NULL, view_key TEXT NOT NULL, revision BIGINT NOT NULL,
 action TEXT NOT NULL CHECK(action IN ('activate','deactivate','retire')),
 previous_source_id TEXT NOT NULL DEFAULT '', selected_source_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(owner_id,view_key,revision)
);

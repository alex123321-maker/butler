-- Retrieval evidence is DATA. These metadata-only rows grant no source access,
-- task permission or accepted fact; ordinary Broker/catalog/ledger guards apply.
CREATE TABLE memory_retrieval_plans (
 run_id TEXT PRIMARY KEY REFERENCES runs(run_id) ON DELETE CASCADE,
 owner_id TEXT NOT NULL, session_key TEXT NOT NULL, task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL, constraints_revision BIGINT NOT NULL,
 policy_version TEXT NOT NULL, plan_hash TEXT NOT NULL CHECK(length(plan_hash)=64),
 plan_json JSONB NOT NULL CHECK(octet_length(plan_json::text)<=4096),
 calls INTEGER NOT NULL DEFAULT 0 CHECK(calls BETWEEN 0 AND 8),
 searches INTEGER NOT NULL DEFAULT 0 CHECK(searches BETWEEN 0 AND 3),
 returned_bytes INTEGER NOT NULL DEFAULT 0 CHECK(returned_bytes BETWEEN 0 AND 65536),
 deadline TIMESTAMPTZ, state TEXT NOT NULL DEFAULT 'active' CHECK(state IN('active','finished','stopped')),
 evidence_json JSONB NOT NULL DEFAULT '{}' CHECK(octet_length(evidence_json::text)<=16384),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE memory_retrieval_operations (
 effect_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES memory_retrieval_plans(run_id) ON DELETE CASCADE,
 tool_call_id TEXT NOT NULL, tool_name TEXT NOT NULL, args_hash TEXT NOT NULL CHECK(length(args_hash)=64),
 strategy TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'reserved' CHECK(state IN('reserved','observed')),
 result_status TEXT NOT NULL DEFAULT '', returned_bytes INTEGER NOT NULL DEFAULT 0 CHECK(returned_bytes BETWEEN 0 AND 65536),
 result_hash TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(run_id,tool_call_id)
);
CREATE TABLE memory_retrieval_opened (
 read_ref TEXT PRIMARY KEY CHECK(length(read_ref)=69),
 effect_id TEXT NOT NULL REFERENCES memory_retrieval_operations(effect_id) ON DELETE CASCADE,
 source_id TEXT NOT NULL REFERENCES memory_sources(source_id),
 source_revision BIGINT NOT NULL CHECK(source_revision>0), access_revision BIGINT NOT NULL CHECK(access_revision>0),
 content_hash TEXT NOT NULL CHECK(length(content_hash)=64), fragment_key TEXT NOT NULL,
 read_offset INTEGER NOT NULL CHECK(read_offset BETWEEN 0 AND 8388608),
 char_count INTEGER NOT NULL CHECK(char_count BETWEEN 1 AND 8192),
 text_hash TEXT NOT NULL CHECK(length(text_hash)=64)
);
CREATE INDEX memory_retrieval_operations_run ON memory_retrieval_operations(run_id);

-- Independent metadata: memory_claims/source tables arrive in046.
-- No source text, model input or credential bytes are stored here.
CREATE TABLE memory_extraction_dedup (
 owner_id TEXT NOT NULL,
 source_id TEXT NOT NULL,
 source_revision BIGINT NOT NULL CHECK(source_revision>0),
 dedup_key TEXT NOT NULL CHECK(length(dedup_key) BETWEEN 1 AND 128),
 request_hash TEXT NOT NULL CHECK(length(request_hash)=64),
 claim_id TEXT NOT NULL UNIQUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(owner_id,source_id,source_revision,dedup_key)
);
CREATE TABLE memory_extraction_runs (
 extraction_id TEXT PRIMARY KEY,
 source_id TEXT NOT NULL,
 source_revision BIGINT NOT NULL CHECK(source_revision>0),
 pipeline_id TEXT NOT NULL,
 extracted_at TIMESTAMPTZ,
 complete BOOLEAN NOT NULL DEFAULT FALSE,
 claim_ids TEXT[] NOT NULL DEFAULT '{}',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

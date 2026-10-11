-- Content-free references only; originals/transcripts remain catalog-owned.
-- No FK to later migration046/049; services require the complete schema.
CREATE TABLE voice_clips (
 clip_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL,
 role TEXT NOT NULL CHECK(role IN ('owner_command','note','external')),
 source_ref JSONB NOT NULL, original_hash TEXT NOT NULL CHECK(length(original_hash)=64),
 state TEXT NOT NULL CHECK(state IN ('stored','pending','review','partial','failed','cancelled','control_pending','submitted','deleted')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision>0),
 transcript_ref JSONB, reviewed_ref JSONB, job_id TEXT NOT NULL DEFAULT '',
 input_id TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '', control_target JSONB,
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX voice_clips_retention ON voice_clips(expires_at) WHERE state<>'deleted';
CREATE TABLE voice_input_sources (
 input_id TEXT PRIMARY KEY, event_id TEXT NOT NULL UNIQUE, owner_id TEXT NOT NULL,
 session_key TEXT NOT NULL, task_id TEXT NOT NULL DEFAULT '', task_revision BIGINT NOT NULL DEFAULT 0,
 source_ref JSONB NOT NULL, original_ref JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

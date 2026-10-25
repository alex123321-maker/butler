-- Content-free kernel lifecycle notices, separate from model publications.
CREATE TABLE execution_notice_bindings (
 binding_id TEXT NOT NULL REFERENCES communication_bindings(binding_id),
 binding_version BIGINT NOT NULL CHECK(binding_version>0),
 owner_id TEXT NOT NULL, activated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(binding_id,binding_version)
);
CREATE TABLE execution_notices (
 notice_id TEXT PRIMARY KEY, sequence BIGINT GENERATED ALWAYS AS IDENTITY UNIQUE,
 input_id TEXT NOT NULL REFERENCES accepted_inputs(input_id) ON DELETE CASCADE,
 owner_id TEXT NOT NULL, session_key TEXT NOT NULL REFERENCES sessions(session_key) ON DELETE CASCADE,
 run_id TEXT REFERENCES runs(run_id) ON DELETE CASCADE,
 binding_id TEXT NOT NULL, binding_version BIGINT NOT NULL,
 channel TEXT NOT NULL CHECK(channel IN ('web','telegram')), recipient_snapshot TEXT NOT NULL,
 code TEXT NOT NULL CHECK(code IN ('queued','working','waiting_for_user','approval_required','blocked','failed','cancelled','missing_response','finished')),
 source_updated_at TIMESTAMPTZ NOT NULL,
 reason TEXT NOT NULL CHECK(reason IN ('','budget','timeout','uncertain_action','source_unavailable','execution_failed')),
 approval_ref TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('available','pending','sending','sent','failed','delivery_unknown','superseded')),
 attempt_count INT NOT NULL DEFAULT 0, claim_id TEXT NOT NULL DEFAULT '', claim_until TIMESTAMPTZ,
 channel_ack TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '',
 next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 deadline_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()+interval '24 hours',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(binding_id,binding_version) REFERENCES execution_notice_bindings(binding_id,binding_version),
 UNIQUE(input_id,binding_id,binding_version,source_updated_at,code,reason,approval_ref)
);
CREATE INDEX execution_notices_pending ON execution_notices(binding_id,binding_version,sequence) WHERE status IN ('pending','sending');

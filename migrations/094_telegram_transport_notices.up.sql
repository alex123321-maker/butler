-- Fixed service receipts/clarifications contain no model or source content.
-- A committed sending claim precedes HTTP; unresolved claims never auto-resend.
CREATE TABLE communication_telegram_notices (
 bot_id BIGINT NOT NULL CHECK(bot_id>0), update_id BIGINT NOT NULL CHECK(update_id>=0),
 binding_id TEXT NOT NULL REFERENCES communication_bindings(binding_id) ON DELETE CASCADE,
 binding_version BIGINT NOT NULL CHECK(binding_version>0),
 code TEXT NOT NULL CHECK(code IN ('target_required','unsupported_input','interaction_required','reply_target_unavailable')),
 status TEXT NOT NULL CHECK(status IN ('sending','sent','delivery_unknown','failed')),
 channel_ack TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(bot_id,update_id)
);

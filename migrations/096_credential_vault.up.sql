ALTER TABLE server_browser_sessions ADD COLUMN observed_url TEXT NOT NULL DEFAULT '';
CREATE TABLE credential_vault (
 id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, alias TEXT NOT NULL, title TEXT NOT NULL,
 site_hint TEXT NOT NULL DEFAULT '', account_label TEXT NOT NULL DEFAULT '',
 username_cipher TEXT NOT NULL, password_cipher TEXT NOT NULL, login_index TEXT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 UNIQUE(owner_id,alias)
);
CREATE INDEX credential_vault_login ON credential_vault(owner_id,login_index);
CREATE TABLE credential_vault_pins (
 owner_id TEXT PRIMARY KEY, salt TEXT NOT NULL, verifier TEXT NOT NULL,
 failures INTEGER NOT NULL DEFAULT 0, blocked_until TIMESTAMPTZ NOT NULL DEFAULT now(), revision BIGINT NOT NULL DEFAULT 1
);
CREATE TABLE credential_vault_unlocks (
 token_hash TEXT PRIMARY KEY, owner_id TEXT NOT NULL REFERENCES credential_vault_pins(owner_id) ON DELETE CASCADE,
 pin_revision BIGINT NOT NULL, expires_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE credential_vault_requests (
 id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, session_key TEXT NOT NULL, run_id TEXT NOT NULL,
 task_id TEXT NOT NULL DEFAULT '', task_revision BIGINT NOT NULL, constraints_revision BIGINT NOT NULL,
 tool_call_id TEXT NOT NULL, kind TEXT NOT NULL CHECK(kind IN ('access','create','otp')),
 parent_request_id TEXT NOT NULL DEFAULT '',
 record_id TEXT NOT NULL DEFAULT '', record_revision BIGINT NOT NULL DEFAULT 0,
 target_origin TEXT NOT NULL, browser_session_id TEXT NOT NULL, purpose TEXT NOT NULL,
 fields JSONB NOT NULL, state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','denied','expired','consumed')),
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 notice_state TEXT NOT NULL DEFAULT 'pending' CHECK(notice_state IN ('pending','sending','sent','failed','unknown')),
 notice_ack TEXT NOT NULL DEFAULT '', notice_phase TEXT NOT NULL DEFAULT '',
 notice_started_at TIMESTAMPTZ,
 UNIQUE(run_id,tool_call_id)
);
CREATE INDEX credential_vault_pending ON credential_vault_requests(owner_id,state,created_at);
CREATE TABLE credential_vault_otps (
 request_id TEXT PRIMARY KEY REFERENCES credential_vault_requests(id) ON DELETE CASCADE,
 cipher TEXT NOT NULL, expires_at TIMESTAMPTZ NOT NULL, claimed_call TEXT NOT NULL DEFAULT '',
 source_device_id TEXT NOT NULL DEFAULT '',source_route_revision BIGINT NOT NULL DEFAULT 0
);
CREATE TABLE credential_vault_audit (
 id BIGSERIAL PRIMARY KEY, owner_id TEXT NOT NULL, record_id TEXT NOT NULL DEFAULT '',
 request_id TEXT NOT NULL DEFAULT '', operation TEXT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE credential_vault_sms_routes (
 record_id TEXT PRIMARY KEY REFERENCES credential_vault(id) ON DELETE CASCADE,
 owner_id TEXT NOT NULL, device_id TEXT NOT NULL, package_name TEXT NOT NULL,
 sender TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT FALSE,revision BIGINT NOT NULL DEFAULT 1
);
CREATE TABLE credential_vault_sms_receipts (
 device_id TEXT NOT NULL,event_id TEXT NOT NULL,request_id TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),PRIMARY KEY(device_id,event_id)
);

ALTER TABLE credential_vault_requests DROP CONSTRAINT credential_vault_requests_kind_check;
ALTER TABLE credential_vault_requests ADD CONSTRAINT credential_vault_requests_kind_check CHECK(kind IN ('access','create','otp','login'));
ALTER TABLE credential_vault_requests ADD COLUMN login_scope JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE credential_vault_requests ADD COLUMN login_claimed_call TEXT NOT NULL DEFAULT '';

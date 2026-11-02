DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM credential_vault_requests WHERE kind='login') THEN
  RAISE EXCEPTION 'login request history exists; refusing destructive rollback';
 END IF;
END $$;
ALTER TABLE credential_vault_requests DROP COLUMN login_claimed_call;
ALTER TABLE credential_vault_requests DROP COLUMN login_scope;
ALTER TABLE credential_vault_requests DROP CONSTRAINT credential_vault_requests_kind_check;
ALTER TABLE credential_vault_requests ADD CONSTRAINT credential_vault_requests_kind_check CHECK(kind IN ('access','create','otp'));

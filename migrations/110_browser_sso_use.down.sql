DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM credential_sso_uses) OR EXISTS(SELECT 1 FROM credential_sso_notices WHERE notice_state<>'pending' OR notice_ack<>'' OR notice_started_at IS NOT NULL) THEN
  RAISE EXCEPTION 'retained SSO use prevents downgrade';
 END IF;
END $$;
DROP VIEW credential_sso_state;
DROP TABLE credential_sso_notices;
DROP TABLE credential_sso_uses;
DROP FUNCTION retain_credential_sso_use();

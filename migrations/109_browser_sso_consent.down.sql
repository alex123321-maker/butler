DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM credential_sso_requests) THEN
  RAISE EXCEPTION 'retained SSO consent prevents downgrade';
 END IF;
END $$;
DROP TABLE credential_sso_requests;
DROP FUNCTION retain_credential_sso_request();

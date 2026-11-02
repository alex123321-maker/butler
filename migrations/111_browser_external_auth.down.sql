DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM browser_external_auth_requests)
 OR EXISTS(SELECT 1 FROM browser_external_auth_uses)
 OR EXISTS(SELECT 1 FROM browser_external_auth_identities) THEN
  RAISE EXCEPTION 'cannot discard retained external auth history';
 END IF;
END $$;
DROP VIEW browser_external_auth_state;
DROP TABLE browser_external_auth_notices,browser_external_auth_closures,browser_external_auth_results,browser_external_auth_verified_identities,browser_external_auth_exchanges,browser_external_auth_launches,browser_external_auth_uses,browser_external_auth_identities,browser_external_auth_requests,browser_external_auth_profiles;
DROP FUNCTION retain_external_auth_fact();
DROP FUNCTION retain_external_auth_request();
DROP FUNCTION retain_external_auth_profile();

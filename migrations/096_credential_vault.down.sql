DROP TABLE IF EXISTS credential_vault_audit;
DROP TABLE IF EXISTS credential_vault_sms_receipts;
DROP TABLE IF EXISTS credential_vault_sms_routes;
DROP TABLE IF EXISTS credential_vault_otps;
DROP TABLE IF EXISTS credential_vault_requests;
DROP TABLE IF EXISTS credential_vault_unlocks;
DROP TABLE IF EXISTS credential_vault_pins;
DROP TABLE IF EXISTS credential_vault;
ALTER TABLE server_browser_sessions DROP COLUMN IF EXISTS observed_url;

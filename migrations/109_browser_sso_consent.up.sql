-- Deliberately separate from legacy credential request aliases: no existing
-- resolver/tool can spend an SSO consent as same-origin login or preparation.
CREATE TABLE credential_sso_requests (
 id TEXT PRIMARY KEY CHECK(id ~ '^ssoq_[0-9a-f]{64}$'),
 owner_id TEXT NOT NULL,
 session_key TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL,
 constraints_revision BIGINT NOT NULL,
 tool_call_id TEXT NOT NULL,
 record_id TEXT NOT NULL,
 record_revision BIGINT NOT NULL CHECK(record_revision>0),
 browser_session_id TEXT NOT NULL REFERENCES server_browser_sessions(browser_session_id),
 rp_page_id TEXT NOT NULL,
 context_epoch BIGINT NOT NULL CHECK(context_epoch>=0),
 scope JSONB NOT NULL CHECK(jsonb_typeof(scope)='object'),
 scope_digest TEXT NOT NULL CHECK(scope_digest ~ '^[0-9a-f]{64}$'),
 purpose TEXT NOT NULL CHECK(length(purpose) BETWEEN 1 AND 500),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','denied')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL,
 UNIQUE(run_id,tool_call_id),
 FOREIGN KEY(browser_session_id,rp_page_id) REFERENCES server_browser_pages(browser_session_id,page_id),
 CHECK(expires_at>created_at)
);
CREATE INDEX credential_sso_pending ON credential_sso_requests(owner_id,created_at) WHERE state='pending';
CREATE FUNCTION retain_credential_sso_request() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'UPDATE' THEN RAISE EXCEPTION 'SSO consent identity must be retained'; END IF;
 IF OLD.state<>'pending' OR NEW.state NOT IN ('approved','denied')
  OR (to_jsonb(NEW)-'state') IS DISTINCT FROM (to_jsonb(OLD)-'state') THEN
  RAISE EXCEPTION 'invalid SSO consent transition';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER retain_sso_request_update BEFORE UPDATE ON credential_sso_requests
 FOR EACH ROW EXECUTE FUNCTION retain_credential_sso_request();
CREATE TRIGGER retain_sso_request_delete BEFORE DELETE OR TRUNCATE ON credential_sso_requests
 FOR EACH STATEMENT EXECUTE FUNCTION retain_credential_sso_request();

CREATE TABLE credential_sso_uses (
 request_id TEXT PRIMARY KEY REFERENCES credential_sso_requests(id),
 run_id TEXT NOT NULL,
 tool_call_id TEXT NOT NULL,
 args_hash TEXT NOT NULL CHECK(args_hash ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(run_id,tool_call_id)
);
CREATE FUNCTION retain_credential_sso_use() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'SSO use claim is immutable and retained'; END $$;
CREATE TRIGGER retain_sso_use BEFORE UPDATE OR DELETE OR TRUNCATE ON credential_sso_uses
 FOR EACH STATEMENT EXECUTE FUNCTION retain_credential_sso_use();

CREATE TABLE credential_sso_notices (
 request_id TEXT PRIMARY KEY REFERENCES credential_sso_requests(id),
 notice_state TEXT NOT NULL DEFAULT 'pending' CHECK(notice_state IN ('pending','sending','sent','failed','unknown')),
 notice_ack TEXT NOT NULL DEFAULT '',
 notice_phase TEXT NOT NULL DEFAULT '',
 notice_started_at TIMESTAMPTZ
);
INSERT INTO credential_sso_notices(request_id) SELECT id FROM credential_sso_requests;
CREATE VIEW credential_sso_state AS
 SELECT q.*, CASE
 WHEN EXISTS(SELECT 1 FROM credential_sso_uses u WHERE u.request_id=q.id) THEN 'consumed'
 WHEN q.expires_at<=clock_timestamp()
 OR NOT EXISTS(SELECT 1 FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=q.run_id AND r.session_key=q.session_key AND s.user_id=q.owner_id AND r.current_state NOT IN ('completed','failed','cancelled','timed_out'))
 OR NOT EXISTS(SELECT 1 FROM credential_vault c WHERE c.id=q.record_id AND c.owner_id=q.owner_id AND c.revision=q.record_revision AND c.status='active')
 OR NOT EXISTS(SELECT 1 FROM server_browser_sessions b JOIN server_browser_pages p USING(browser_session_id)
  WHERE b.browser_session_id=q.browser_session_id AND b.run_id=q.run_id AND b.owner_id=q.owner_id AND b.task_id=q.task_id
  AND b.page_mode AND b.mode='agent' AND b.epoch=q.context_epoch AND b.expires_at>clock_timestamp()
  AND p.page_id=q.rp_page_id AND p.document_id=q.scope->'rp_page'->>'document_id'
  AND to_jsonb(p.observation_version)=q.scope->'rp_page'->'observation_version'
  AND p.observed_epoch=b.epoch AND p.observed_url=q.scope->>'rp_url' AND p.state='ready')
 OR (q.task_id<>'' AND NOT EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=q.task_id AND t.owner_id=q.owner_id AND t.revision=q.task_revision AND t.constraints_revision=q.constraints_revision AND t.status NOT IN ('completed','cancelled','paused','blocked')))
 THEN 'expired' ELSE q.state END AS live_state
 FROM credential_sso_requests q;

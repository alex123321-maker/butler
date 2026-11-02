-- External Chrome OAuth is a separate owner-controlled authentication flow.
-- Neither native-form grants nor generic credential aliases can consume it.
CREATE TABLE browser_external_auth_profiles (
 fingerprint TEXT PRIMARY KEY CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 profile JSONB NOT NULL CHECK(jsonb_typeof(profile)='object'),
 active BOOLEAN NOT NULL DEFAULT TRUE,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE UNIQUE INDEX external_auth_one_profile ON browser_external_auth_profiles(active) WHERE active;
CREATE FUNCTION retain_external_auth_profile() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF (to_jsonb(NEW)-'active') IS DISTINCT FROM (to_jsonb(OLD)-'active') THEN
  RAISE EXCEPTION 'external auth profile identity is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER external_auth_profile_identity BEFORE UPDATE ON browser_external_auth_profiles
 FOR EACH ROW EXECUTE FUNCTION retain_external_auth_profile();

CREATE TABLE browser_external_auth_requests (
 id TEXT PRIMARY KEY CHECK(id ~ '^extq_[0-9a-f]{64}$'),
 owner_id TEXT NOT NULL,
 session_key TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 input_id TEXT NOT NULL REFERENCES accepted_inputs(input_id),
 task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL,
 constraints_revision BIGINT NOT NULL,
 tool_call_id TEXT NOT NULL,
 source_action_id TEXT NOT NULL REFERENCES action_intents(action_id),
 record_id TEXT NOT NULL,
 record_revision BIGINT NOT NULL CHECK(record_revision>0),
 browser_session_id TEXT NOT NULL REFERENCES server_browser_sessions(browser_session_id),
 rp_page_id TEXT NOT NULL,
 context_epoch BIGINT NOT NULL CHECK(context_epoch>=0),
 profile_fingerprint TEXT NOT NULL REFERENCES browser_external_auth_profiles(fingerprint),
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
CREATE INDEX external_auth_pending ON browser_external_auth_requests(owner_id,created_at) WHERE state='pending';
CREATE FUNCTION retain_external_auth_request() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP<>'UPDATE' THEN RAISE EXCEPTION 'external auth request must be retained'; END IF;
 IF OLD.state<>'pending' OR NEW.state NOT IN ('approved','denied')
  OR (to_jsonb(NEW)-'state') IS DISTINCT FROM (to_jsonb(OLD)-'state') THEN
  RAISE EXCEPTION 'invalid external auth decision';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER external_auth_request_update BEFORE UPDATE ON browser_external_auth_requests
 FOR EACH ROW EXECUTE FUNCTION retain_external_auth_request();
CREATE TRIGGER external_auth_request_delete BEFORE DELETE OR TRUNCATE ON browser_external_auth_requests
 FOR EACH STATEMENT EXECUTE FUNCTION retain_external_auth_request();

CREATE TABLE browser_external_auth_uses (
 request_id TEXT PRIMARY KEY REFERENCES browser_external_auth_requests(id),
 state_hash TEXT NOT NULL UNIQUE CHECK(state_hash ~ '^[0-9a-f]{64}$'),
 ticket_hash TEXT NOT NULL UNIQUE CHECK(ticket_hash ~ '^[0-9a-f]{64}$'),
 cookie_hash TEXT NOT NULL UNIQUE CHECK(cookie_hash ~ '^[0-9a-f]{64}$'),
 private_cipher TEXT NOT NULL CHECK(length(private_cipher)>0),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE browser_external_auth_launches (
 request_id TEXT PRIMARY KEY REFERENCES browser_external_auth_uses(request_id),
 launched_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE browser_external_auth_exchanges (
 request_id TEXT PRIMARY KEY REFERENCES browser_external_auth_launches(request_id),
 effect_id TEXT NOT NULL UNIQUE REFERENCES action_effects(action_id),
 claimed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE browser_external_auth_results (
 request_id TEXT PRIMARY KEY REFERENCES browser_external_auth_exchanges(request_id),
 outcome TEXT NOT NULL CHECK(outcome IN ('verified','unverified','unknown')),
 rp_authenticated_read BOOLEAN NOT NULL DEFAULT FALSE,
 rp_anonymous_challenge BOOLEAN NOT NULL DEFAULT FALSE,
 rp_reference TEXT NOT NULL DEFAULT '' CHECK(length(rp_reference)<=256),
 finished_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK((outcome='verified')=(rp_authenticated_read AND rp_anonymous_challenge)),
 CHECK((outcome='verified')=(length(rp_reference)>0))
);
-- This is an independently generated test-RP page reference, not a token,
-- provider subject, email or grant. No access/refresh/ID token is retained.
CREATE TABLE browser_external_auth_verified_identities (
 request_id TEXT PRIMARY KEY REFERENCES browser_external_auth_exchanges(request_id),
 private_reference TEXT NOT NULL CHECK(length(private_reference) BETWEEN 1 AND 256),
 expires_at TIMESTAMPTZ NOT NULL,
 verified_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK(expires_at>verified_at)
);
-- Identity matching never stores a raw provider subject or email. A verified
-- subject becomes a stable owner/account/client keyed binding, across edits.
CREATE TABLE browser_external_auth_identities (
 owner_id TEXT NOT NULL,
 record_id TEXT NOT NULL,
 profile_fingerprint TEXT NOT NULL REFERENCES browser_external_auth_profiles(fingerprint),
 subject_index TEXT NOT NULL CHECK(subject_index ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(owner_id,record_id,profile_fingerprint)
);
CREATE TABLE browser_external_auth_notices (
 request_id TEXT PRIMARY KEY REFERENCES browser_external_auth_requests(id),
 notice_state TEXT NOT NULL DEFAULT 'pending' CHECK(notice_state IN ('pending','sending','sent','unknown')),
 notice_ack TEXT NOT NULL DEFAULT '',
 notice_phase TEXT NOT NULL DEFAULT '',
 notice_started_at TIMESTAMPTZ
);
-- Closing authority is independent of an already-issued provider request.
-- It neither erases an exchange nor turns an ambiguous effect into absence.
CREATE TABLE browser_external_auth_closures (
 request_id TEXT PRIMARY KEY REFERENCES browser_external_auth_requests(id),
 reason TEXT NOT NULL CHECK(reason IN ('cancelled','rejected')),
 closed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE VIEW browser_external_auth_state AS
 SELECT q.*, CASE
 WHEN q.state='denied' THEN 'denied'
 WHEN r.request_id IS NOT NULL THEN r.outcome
 WHEN z.request_id IS NOT NULL THEN z.reason
 WHEN e.request_id IS NOT NULL THEN 'exchanging'
 WHEN q.expires_at<=clock_timestamp() THEN 'expired'
 WHEN NOT EXISTS(SELECT 1 FROM browser_external_auth_profiles f WHERE f.fingerprint=q.profile_fingerprint AND f.active)
  OR NOT EXISTS(SELECT 1 FROM credential_vault c WHERE c.id=q.record_id AND c.owner_id=q.owner_id AND c.revision=q.record_revision AND c.status='active')
  OR NOT EXISTS(SELECT 1 FROM runs x JOIN sessions s USING(session_key) WHERE x.run_id=q.run_id AND s.session_key=q.session_key AND s.user_id=q.owner_id AND x.current_state NOT IN ('completed','failed','cancelled','timed_out'))
  OR NOT EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=q.task_id AND t.owner_id=q.owner_id AND t.revision=q.task_revision AND t.constraints_revision=q.constraints_revision AND t.status NOT IN ('completed','cancelled','paused','blocked'))
  OR NOT EXISTS(SELECT 1 FROM action_intents a JOIN action_effects e ON e.action_id=a.effect_id JOIN action_effect_dispatches d ON d.execution_action_id=a.action_id AND d.authorization_id=e.authorization_id WHERE a.action_id=q.source_action_id AND a.run_id=q.run_id AND a.tool_call_id=q.tool_call_id AND e.state='effect_confirmed' AND d.outcome='dispatch_completed')
  OR NOT EXISTS(SELECT 1 FROM server_browser_sessions b JOIN server_browser_pages p ON p.browser_session_id=b.browser_session_id WHERE b.browser_session_id=q.browser_session_id AND b.owner_id=q.owner_id AND b.run_id=q.run_id AND b.task_id=q.task_id AND b.page_mode AND b.mode='agent' AND b.epoch=q.context_epoch AND b.expires_at>clock_timestamp() AND p.page_id=q.rp_page_id AND p.document_id=q.scope->'rp_page'->>'document_id' AND p.observation_version=(q.scope->'rp_page'->>'observation_version')::bigint AND p.observed_epoch=q.context_epoch AND p.observed_url=q.scope->>'rp_url' AND p.state='ready')
 THEN 'stale'
 WHEN l.request_id IS NOT NULL THEN 'launched'
 ELSE q.state END AS live_state
 FROM browser_external_auth_requests q
 LEFT JOIN browser_external_auth_results r ON r.request_id=q.id
 LEFT JOIN browser_external_auth_closures z ON z.request_id=q.id
 LEFT JOIN browser_external_auth_exchanges e ON e.request_id=q.id
 LEFT JOIN browser_external_auth_launches l ON l.request_id=q.id;
CREATE FUNCTION retain_external_auth_fact() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'external auth fact is append-only'; END $$;
DO $$ DECLARE t TEXT; BEGIN
 FOREACH t IN ARRAY ARRAY['browser_external_auth_uses','browser_external_auth_launches','browser_external_auth_exchanges','browser_external_auth_results','browser_external_auth_verified_identities','browser_external_auth_identities','browser_external_auth_closures'] LOOP
  EXECUTE format('CREATE TRIGGER retain_fact_mutation BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION retain_external_auth_fact()',t);
  EXECUTE format('CREATE TRIGGER retain_fact_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION retain_external_auth_fact()',t);
 END LOOP;
END $$;

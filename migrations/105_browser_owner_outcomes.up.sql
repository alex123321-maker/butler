CREATE TABLE browser_owner_outcome_reviews (
 review_id TEXT PRIMARY KEY,
 effect_id TEXT NOT NULL REFERENCES action_effects(action_id),
 fingerprint TEXT NOT NULL,
 execution_action_id TEXT NOT NULL REFERENCES action_intents(action_id),
 owner_id TEXT NOT NULL,
 run_id TEXT NOT NULL REFERENCES runs(run_id),
 session_key TEXT NOT NULL REFERENCES sessions(session_key),
 tool_call_id TEXT NOT NULL,
 browser_session_id TEXT NOT NULL,
 task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL,
 constraints_revision BIGINT NOT NULL,
 observation_call_id TEXT NOT NULL DEFAULT '',
 source_kind TEXT NOT NULL CHECK(source_kind IN ('live_frozen','historical_expired')),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','effect_confirmed','effect_absent','unresolved')),
 revision BIGINT NOT NULL DEFAULT 1 CHECK(revision=1),
 resource_hint TEXT NOT NULL,
 decision_digest TEXT NOT NULL DEFAULT '',
 delivery_digest TEXT NOT NULL DEFAULT '',
 receipt_digest TEXT NOT NULL DEFAULT '',
 evidence_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL,
 decided_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX browser_outcome_one_pending ON browser_owner_outcome_reviews(effect_id) WHERE state='pending';
CREATE INDEX browser_outcome_owner_pending ON browser_owner_outcome_reviews(owner_id,created_at) WHERE state='pending';
CREATE FUNCTION retain_browser_owner_outcome() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' OR OLD.state<>'pending' THEN RAISE EXCEPTION 'owner outcome reports are retained'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER retain_browser_outcome BEFORE UPDATE OR DELETE ON browser_owner_outcome_reviews
 FOR EACH ROW EXECUTE FUNCTION retain_browser_owner_outcome();

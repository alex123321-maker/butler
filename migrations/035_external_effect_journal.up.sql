CREATE TABLE action_effects (
 action_id TEXT PRIMARY KEY,
 fingerprint TEXT NOT NULL,
 initial_binding JSONB NOT NULL,
 policy JSONB NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('prepared','allowed','executing','effect_confirmed','effect_absent','effect_unknown')),
 idempotency_key TEXT NOT NULL DEFAULT '',
 authorization_id TEXT NOT NULL DEFAULT '',
 dispatch_outcome TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE action_intents ADD COLUMN effect_id TEXT;
ALTER TABLE action_intents ADD COLUMN effect_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE action_intents ADD COLUMN effect_policy JSONB NOT NULL DEFAULT '{"retry_safety":"unsafe","verification_method":"none"}'::jsonb;
INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,authorization_id,dispatch_outcome,created_at)
 SELECT i.action_id,i.action_hash,i.binding,i.effect_policy,
 CASE WHEN a.state='authorized' THEN 'allowed' WHEN a.state='inflight' THEN 'executing'
 WHEN a.state IN ('dispatch_completed','unknown') THEN 'effect_unknown' ELSE 'prepared' END,
 COALESCE(a.authorization_id,''),COALESCE(a.state,''),i.created_at
 FROM action_intents i LEFT JOIN action_authorizations a ON a.action_id=i.action_id;
UPDATE action_intents SET effect_id=action_id,effect_fingerprint=action_hash;
ALTER TABLE action_intents ALTER COLUMN effect_id SET NOT NULL;
ALTER TABLE action_intents ADD FOREIGN KEY(effect_id) REFERENCES action_effects(action_id);
CREATE INDEX action_intents_effect ON action_intents(effect_id);

CREATE TABLE action_effect_evidence (
 evidence_id TEXT PRIMARY KEY,
 action_id TEXT NOT NULL REFERENCES action_effects(action_id),
 source_action_id TEXT NOT NULL REFERENCES action_intents(action_id),
 evidence JSONB NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX action_effect_evidence_action ON action_effect_evidence(action_id,recorded_at,evidence_id);

CREATE TABLE action_effect_dispatches (
 authorization_id TEXT PRIMARY KEY,
 action_id TEXT NOT NULL REFERENCES action_effects(action_id),
 execution_action_id TEXT NOT NULL REFERENCES action_intents(action_id),
 outcome TEXT NOT NULL CHECK(outcome IN ('executing','dispatch_completed','unknown')),
 dispatched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 finished_at TIMESTAMPTZ
);
INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome,dispatched_at,finished_at)
 SELECT a.authorization_id,a.action_id,a.action_id,CASE WHEN a.state='inflight' THEN 'executing' ELSE a.state END,
 COALESCE(a.dispatched_at,a.created_at),a.finished_at
 FROM action_authorizations a WHERE a.state IN ('inflight','dispatch_completed','unknown');

CREATE FUNCTION reject_effect_evidence_mutation() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'external effect evidence is append-only'; END;
$$;
CREATE TRIGGER effect_evidence_immutable BEFORE UPDATE OR DELETE ON action_effect_evidence
 FOR EACH ROW EXECUTE FUNCTION reject_effect_evidence_mutation();

-- Orchestrator owns accepted capability identity. No raw Catalog body or secret.
CREATE TABLE delegation_admission_snapshots (
 task_id TEXT PRIMARY KEY REFERENCES task_delegations(task_id),
 owner_id TEXT NOT NULL,
 parent_task_id TEXT NOT NULL REFERENCES tasks(task_id),
 parent_revision BIGINT NOT NULL CHECK(parent_revision>0),
 parent_constraints_revision BIGINT NOT NULL CHECK(parent_constraints_revision>0),
 execution_action_id TEXT NOT NULL REFERENCES action_intents(action_id),
 effect_id TEXT NOT NULL UNIQUE REFERENCES action_effects(action_id),
 authorization_id TEXT NOT NULL REFERENCES action_authorizations(authorization_id),
 action_hash TEXT NOT NULL CHECK(length(action_hash)=64),
 args_hash TEXT NOT NULL CHECK(length(args_hash)=64),
 contract_hash TEXT NOT NULL CHECK(length(contract_hash)=64),
 accepted_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION reject_delegation_admission_mutation() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'accepted delegation capabilities are immutable';
END $$;
CREATE TRIGGER delegation_admission_immutable BEFORE UPDATE OR DELETE ON delegation_admission_snapshots
 FOR EACH ROW EXECUTE FUNCTION reject_delegation_admission_mutation();

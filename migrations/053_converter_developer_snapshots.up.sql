-- Accepted developer capabilities contain identities/hashes only; all selected
-- spec/package bytes remain deletion-linked Catalog DATA. No FK to later057.
CREATE TABLE converter_developer_snapshots (
 task_id TEXT PRIMARY KEY REFERENCES tasks(task_id),
 owner_id TEXT NOT NULL,
 parent_task_id TEXT NOT NULL REFERENCES tasks(task_id),
 parent_revision BIGINT NOT NULL CHECK(parent_revision>0),
 parent_constraints_revision BIGINT NOT NULL CHECK(parent_constraints_revision>0),
 execution_action_id TEXT NOT NULL REFERENCES action_intents(action_id),
 effect_id TEXT NOT NULL UNIQUE REFERENCES action_effects(action_id),
 authorization_id TEXT NOT NULL REFERENCES action_authorizations(authorization_id),
 action_hash TEXT NOT NULL,
 args_hash TEXT NOT NULL,
 contract_hash TEXT NOT NULL,
 snapshot_hash TEXT NOT NULL,
 accepted_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER converter_developer_immutable BEFORE UPDATE OR DELETE ON converter_developer_snapshots
 FOR EACH ROW EXECUTE FUNCTION converter_immutable_record();

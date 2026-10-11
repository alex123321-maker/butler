-- Exact private metadata for optional Broker workspace admission. No body,
-- host root, credential, reusable service token or model verdict is stored.
CREATE TABLE code_workspace_authorizations (
 authorization_id TEXT PRIMARY KEY REFERENCES action_authorizations(authorization_id),
 profile_id TEXT NOT NULL,
 workspace_id TEXT NOT NULL CHECK(workspace_id ~ '^[a-f0-9]{32}$'),
 tool_name TEXT NOT NULL CHECK(tool_name IN ('workspace.create','workspace.read','workspace.write')),
 contract_hash TEXT NOT NULL CHECK(contract_hash ~ '^[a-f0-9]{64}$'),
 args_hash TEXT NOT NULL CHECK(args_hash ~ '^[a-f0-9]{64}$'),
 source_context JSONB NOT NULL,
 path TEXT NOT NULL,
 body_sha256 TEXT NOT NULL,
 body_bytes BIGINT NOT NULL CHECK(body_bytes>=0 AND body_bytes<=65536),
 expected_snapshot JSONB NOT NULL,
 input_claim_hash TEXT NOT NULL CHECK(input_claim_hash ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION butler_workspace_authorization_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'prepared workspace authorization is immutable'; END;
$$;
CREATE TRIGGER code_workspace_authorizations_immutable BEFORE UPDATE OR DELETE ON code_workspace_authorizations
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_authorization_immutable();

CREATE TABLE code_workspace_dispatches (
 ticket_id TEXT PRIMARY KEY CHECK(ticket_id ~ '^[a-f0-9]{48}$'),
 authorization_id TEXT NOT NULL UNIQUE REFERENCES code_workspace_authorizations(authorization_id),
 effect_id TEXT NOT NULL UNIQUE REFERENCES action_effects(action_id),
 resource_id TEXT NOT NULL REFERENCES execution_resources(resource_id),
 resource_lease_id TEXT NOT NULL UNIQUE REFERENCES execution_resource_leases(lease_id),
 resource_version BIGINT NOT NULL CHECK(resource_version>0),
 resource_fence BIGINT NOT NULL CHECK(resource_fence>0),
 deadline TIMESTAMPTZ NOT NULL,
 phase TEXT NOT NULL DEFAULT 'prepared' CHECK(phase IN ('prepared','running','completed','unknown')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 finished_at TIMESTAMPTZ,
 CHECK(deadline>created_at)
);
CREATE TABLE code_workspace_effect_evidence (
 ticket_id TEXT PRIMARY KEY REFERENCES code_workspace_dispatches(ticket_id),
 source_id TEXT REFERENCES memory_sources(source_id),
 observation JSONB NOT NULL,
 observation_hash TEXT NOT NULL CHECK(observation_hash ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER code_workspace_evidence_immutable BEFORE UPDATE OR DELETE ON code_workspace_effect_evidence
 FOR EACH ROW EXECUTE FUNCTION butler_workspace_authorization_immutable();

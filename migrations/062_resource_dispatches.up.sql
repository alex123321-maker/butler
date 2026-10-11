-- Private provenance for runtime-created controlled browser sessions. Model
-- identifiers alone cannot allocate/claim an unrelated live browser resource.
CREATE TABLE execution_browser_resources (
 profile_id TEXT NOT NULL,
 session_hash TEXT NOT NULL,
 resource_id TEXT NOT NULL UNIQUE,
 owner_id TEXT NOT NULL,
 run_id TEXT NOT NULL,
 task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 created_by_effect TEXT NOT NULL REFERENCES action_effects(action_id),
 expires_at TIMESTAMPTZ NOT NULL,
 closed_at TIMESTAMPTZ,
 PRIMARY KEY(profile_id,session_hash)
);
CREATE TABLE execution_resource_dispatches (
 ticket_id TEXT PRIMARY KEY,
 authorization_id TEXT NOT NULL UNIQUE REFERENCES action_authorizations(authorization_id),
 action_id TEXT NOT NULL,
 effect_id TEXT NOT NULL REFERENCES action_effects(action_id),
 owner_id TEXT NOT NULL,
 run_id TEXT NOT NULL,
 session_key TEXT NOT NULL,
 task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 constraints_revision BIGINT NOT NULL CHECK(constraints_revision>0),
 owner_lease_id TEXT NOT NULL,
 owner_epoch BIGINT NOT NULL CHECK(owner_epoch>0),
 resource_id TEXT NOT NULL REFERENCES execution_resources(resource_id),
 resource_lease_id TEXT NOT NULL UNIQUE REFERENCES execution_resource_leases(lease_id),
 resource_version BIGINT NOT NULL CHECK(resource_version>0),
 resource_fence BIGINT NOT NULL CHECK(resource_fence>0),
 profile_id TEXT NOT NULL,
 tool_name TEXT NOT NULL,
 contract_hash TEXT NOT NULL,
 args_hash TEXT NOT NULL,
 deadline TIMESTAMPTZ NOT NULL,
 phase TEXT NOT NULL DEFAULT 'prepared' CHECK(phase IN ('prepared','running','completed','unknown')),
 receipt_hash TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 finished_at TIMESTAMPTZ,
 CHECK(deadline>created_at)
);
CREATE INDEX execution_resource_dispatch_run ON execution_resource_dispatches(run_id,task_id);

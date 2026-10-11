CREATE TABLE execution_resources (
 resource_id TEXT PRIMARY KEY,
 owner_id TEXT NOT NULL,
 version BIGINT NOT NULL DEFAULT 1 CHECK(version>0),
 fence_epoch BIGINT NOT NULL DEFAULT 1 CHECK(fence_epoch>0),
 inflight_lease_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE execution_resource_leases (
 lease_id TEXT PRIMARY KEY,
 resource_id TEXT NOT NULL REFERENCES execution_resources(resource_id),
 owner_id TEXT NOT NULL,
 run_id TEXT NOT NULL,
 session_key TEXT NOT NULL,
 task_id TEXT NOT NULL,
 task_revision BIGINT NOT NULL CHECK(task_revision>0),
 constraints_revision BIGINT NOT NULL CHECK(constraints_revision>0),
 owner_epoch BIGINT NOT NULL CHECK(owner_epoch>0),
 mode TEXT NOT NULL CHECK(mode IN ('read','write')),
 resource_version BIGINT NOT NULL CHECK(resource_version>0),
 fence_epoch BIGINT NOT NULL CHECK(fence_epoch>0),
 state TEXT NOT NULL DEFAULT 'held' CHECK(state IN ('held','claimed','released','completed','unknown','reconciled')),
 expires_at TIMESTAMPTZ NOT NULL,
 evidence_id TEXT NOT NULL DEFAULT '',
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 finished_at TIMESTAMPTZ,
 CHECK(expires_at>created_at)
);
CREATE INDEX execution_resource_live ON execution_resource_leases(resource_id,expires_at) WHERE state IN ('held','claimed');
CREATE INDEX execution_resource_run ON execution_resource_leases(run_id,task_id);

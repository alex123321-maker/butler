CREATE TABLE action_intents (
 action_id TEXT PRIMARY KEY,
 run_id TEXT NOT NULL,
 tool_call_id TEXT NOT NULL,
 action_hash TEXT NOT NULL,
 binding JSONB NOT NULL,
 requires_approval BOOLEAN NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(run_id,tool_call_id)
);
CREATE TABLE action_mandates (
 mandate_id TEXT PRIMARY KEY,
 owner_id TEXT NOT NULL,
 task_id TEXT NOT NULL,
 revision BIGINT NOT NULL CHECK(revision>0),
 mandate_data JSONB NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE action_authorizations (
 authorization_id TEXT PRIMARY KEY,
 action_id TEXT NOT NULL UNIQUE REFERENCES action_intents(action_id),
 action_hash TEXT NOT NULL,
 grant_data JSONB NOT NULL,
 mandate_id TEXT REFERENCES action_mandates(mandate_id),
 mandate_revision BIGINT NOT NULL DEFAULT 0,
 expires_at TIMESTAMPTZ NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('authorized','inflight','dispatch_completed','unknown','revoked')),
 dispatched_at TIMESTAMPTZ,
 finished_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX action_authorizations_mandate ON action_authorizations(mandate_id) WHERE mandate_id IS NOT NULL;

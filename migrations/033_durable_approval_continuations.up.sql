CREATE TABLE approval_actions (
 approval_id TEXT PRIMARY KEY REFERENCES approvals(approval_id) ON DELETE CASCADE,
 action_id TEXT NOT NULL UNIQUE,
 action_hash TEXT NOT NULL,
 task_id TEXT REFERENCES tasks(task_id),
 task_revision BIGINT NOT NULL,
 constraints_revision BIGINT NOT NULL,
 owner_id TEXT NOT NULL,
 channel TEXT NOT NULL CHECK(channel IN ('web','telegram')),
 binding_json JSONB NOT NULL,
 continuation_version INTEGER NOT NULL CHECK(continuation_version=1),
 continuation_json JSONB NOT NULL,
 state TEXT NOT NULL DEFAULT 'waiting' CHECK(state IN ('waiting','ready','dispatching','completed','blocked')),
 decision_input_id TEXT,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE approval_decision_inputs (
 decision_input_id TEXT PRIMARY KEY,
 approval_id TEXT NOT NULL REFERENCES approvals(approval_id) ON DELETE CASCADE,
 source TEXT NOT NULL,
 delivery_id TEXT NOT NULL,
 owner_id TEXT NOT NULL,
 approved BOOLEAN NOT NULL,
 received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(source,delivery_id)
);
-- Legacy waits have no exact action binding or serializable continuation.
-- They remain in history, but cannot grant a new dispatch after upgrade.
UPDATE approvals SET status='expired', resolved_via='system',resolved_by='migration',
 resolution_reason='legacy approval has no durable exact continuation; prepare a new action',
 resolved_at=clock_timestamp(),updated_at=clock_timestamp()
 WHERE status='pending' AND approval_type='tool_call';

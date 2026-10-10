ALTER TABLE tasks ADD COLUMN execution_policy TEXT NOT NULL DEFAULT 'normal'
 CHECK(execution_policy IN ('normal','prepare_only'));
CREATE TABLE task_control_events (
 control_id TEXT PRIMARY KEY,
 task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
 owner_id TEXT NOT NULL,
 source TEXT NOT NULL CHECK(source IN ('web','telegram')),
 delivery_id TEXT NOT NULL,
 kind TEXT NOT NULL,
 request_json JSONB NOT NULL,
 result_json JSONB NOT NULL,
 revision_before BIGINT NOT NULL,
 revision_after BIGINT NOT NULL,
 applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(owner_id,source,delivery_id)
);
CREATE TABLE task_control_targets (
 control_id TEXT NOT NULL REFERENCES task_control_events(control_id) ON DELETE CASCADE,
 task_id TEXT NOT NULL REFERENCES tasks(task_id) ON DELETE CASCADE,
 revision_before BIGINT NOT NULL,
 revision_after BIGINT NOT NULL,
 previous_working_json JSONB NOT NULL,
 PRIMARY KEY(control_id,task_id)
);
CREATE INDEX task_control_events_task_time ON task_control_events(task_id,applied_at);

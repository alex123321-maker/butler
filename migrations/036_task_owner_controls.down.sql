DROP TABLE IF EXISTS task_control_targets;
DROP TABLE IF EXISTS task_control_events;
ALTER TABLE tasks DROP COLUMN IF EXISTS execution_policy;

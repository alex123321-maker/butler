DO $$
BEGIN
 IF EXISTS(SELECT 1 FROM task_control_events) OR EXISTS(SELECT 1 FROM tasks WHERE execution_policy='prepare_only') THEN
  RAISE EXCEPTION 'task owner control history or restrictions exist; downgrade would discard authority';
 END IF;
END $$;
DROP TABLE IF EXISTS task_control_targets;
DROP TABLE IF EXISTS task_control_events;
ALTER TABLE tasks DROP COLUMN IF EXISTS execution_policy;

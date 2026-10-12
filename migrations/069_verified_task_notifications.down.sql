DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM task_verified_notifications) OR EXISTS(SELECT 1 FROM tasks WHERE result_delivery='verified_task')
 THEN RAISE EXCEPTION 'verified task delivery history exists; preserve before rollback'; END IF;
END $$;
DROP TABLE task_verified_notifications;
DROP FUNCTION butler_immutable_verified_task_notification();
DROP TRIGGER immutable_task_result_delivery ON tasks;
DROP FUNCTION butler_immutable_task_result_delivery();
ALTER TABLE tasks DROP COLUMN result_delivery;

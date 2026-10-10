ALTER TABLE tasks DROP CONSTRAINT tasks_completion_result_fk;
DROP TABLE task_results;
ALTER TABLE runs DROP CONSTRAINT runs_task_revision_check;
ALTER TABLE runs DROP COLUMN task_revision;
ALTER TABLE runs DROP COLUMN task_id;
DROP TABLE tasks;

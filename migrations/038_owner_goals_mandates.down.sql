DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM owner_goals) OR EXISTS(SELECT 1 FROM task_authority_bindings)
 THEN RAISE EXCEPTION 'owner authority history exists; preserve and explicitly archive before rollback'; END IF;
END $$;
DROP TABLE owner_authority_jobs;
DROP TABLE owner_authority_events;
DROP TABLE task_authority_bindings;
DROP INDEX action_mandates_goal;
ALTER TABLE action_mandates DROP COLUMN status, DROP COLUMN goal_revision, DROP COLUMN goal_id, DROP COLUMN scope_kind;
DROP TABLE owner_goals;

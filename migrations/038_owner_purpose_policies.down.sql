DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM task_authority_bindings) OR EXISTS(SELECT 1 FROM owner_goals WHERE kind IN ('preference','constraint'))
 THEN RAISE EXCEPTION 'purpose authority history exists; preserve and explicitly archive before rollback'; END IF;
END $$;
ALTER TABLE task_authority_bindings DROP COLUMN policy_revision;
DROP TABLE owner_purpose_policies;

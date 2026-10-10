DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM task_delegations) OR EXISTS(SELECT 1 FROM execution_budget_scopes WHERE kind='delegation') THEN
  RAISE EXCEPTION 'delegation rollback requires preserving durable contracts and budgets';
 END IF;
END $$;
DROP FUNCTION butler_task_check_delegation(TEXT);
DROP TABLE task_delegation_dependencies;
DROP TABLE task_delegations;
ALTER TABLE execution_budget_scopes DROP CONSTRAINT execution_budget_scopes_kind_check;
ALTER TABLE execution_budget_scopes ADD CONSTRAINT execution_budget_scopes_kind_check
 CHECK(kind IN ('task','mandate','owner_day','conversation','auxiliary'));

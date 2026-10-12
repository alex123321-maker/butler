DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM code_workspace_recoveries) OR EXISTS(SELECT 1 FROM code_workspace_executor_bindings) THEN
  RAISE EXCEPTION 'retained workspace executor/recovery history prevents rollback';
 END IF;
END $$;
DROP TABLE code_workspace_erasure_receipts;
DROP TABLE code_workspace_recoveries;
DROP FUNCTION butler_workspace_recovery_immutable();
DROP TABLE code_workspace_executor_bindings;
DROP TRIGGER code_workspace_dispatches_immutable ON code_workspace_dispatches;
DROP FUNCTION butler_workspace_dispatch_immutable();

DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM code_workspace_authorizations) OR EXISTS(SELECT 1 FROM code_workspace_dispatches) OR EXISTS(SELECT 1 FROM code_workspace_effect_evidence) THEN
  RAISE EXCEPTION 'cannot drop prepared workspace dispatch/effect history';
 END IF;
END $$;
DROP TABLE code_workspace_effect_evidence;
DROP TABLE code_workspace_dispatches;
DROP TABLE code_workspace_authorizations;
DROP FUNCTION butler_workspace_authorization_immutable();

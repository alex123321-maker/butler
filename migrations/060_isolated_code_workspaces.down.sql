DO $$ BEGIN
  IF EXISTS (SELECT 1 FROM code_workspaces) OR EXISTS (SELECT 1 FROM code_workspace_files) THEN
    RAISE EXCEPTION 'refuse to discard isolated workspace history';
  END IF;
END $$;
DROP TABLE code_workspace_files;
DROP TABLE code_workspaces;

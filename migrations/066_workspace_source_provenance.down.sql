DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM code_workspace_sources) OR EXISTS(SELECT 1 FROM code_workspace_deletions) THEN
  RAISE EXCEPTION 'cannot drop workspace provenance/deletion history';
 END IF;
END $$;
DROP TRIGGER code_workspace_source_deleted ON memory_sources;
DROP FUNCTION butler_workspace_source_deleted();
DROP TABLE code_workspace_deletions;
DROP TABLE code_workspace_sources;
DROP FUNCTION butler_workspace_source_immutable();

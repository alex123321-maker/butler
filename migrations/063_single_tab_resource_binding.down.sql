DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM execution_resource_dispatches WHERE single_tab_binding IS NOT NULL) OR EXISTS(SELECT 1 FROM guarded_single_tab_requests) OR EXISTS(SELECT 1 FROM guarded_single_tab_selections) THEN
  RAISE EXCEPTION 'single-tab dispatch history cannot be discarded';
 END IF;
END $$;
ALTER TABLE execution_resource_dispatches DROP COLUMN single_tab_binding;
DROP TABLE guarded_single_tab_selections;
DROP TABLE guarded_single_tab_requests;
DROP FUNCTION guarded_single_tab_immutable();

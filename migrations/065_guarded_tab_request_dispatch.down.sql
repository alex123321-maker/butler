DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM guarded_single_tab_dispatches) THEN
  RAISE EXCEPTION 'cannot discard retained guarded request dispatch history';
 END IF;
END $$;
DROP TABLE guarded_single_tab_dispatches;
DROP FUNCTION guarded_tab_dispatch_immutable();

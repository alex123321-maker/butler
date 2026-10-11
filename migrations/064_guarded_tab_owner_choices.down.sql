DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM guarded_single_tab_choices) THEN
  RAISE EXCEPTION 'guarded owner choice history cannot be discarded';
 END IF;
END $$;
DROP TABLE guarded_single_tab_choices;

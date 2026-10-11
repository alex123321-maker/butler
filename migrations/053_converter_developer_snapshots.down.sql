DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM converter_developer_snapshots) THEN
  RAISE EXCEPTION 'retained converter developer admission evidence prevents rollback';
 END IF;
END $$;
DROP TABLE converter_developer_snapshots;

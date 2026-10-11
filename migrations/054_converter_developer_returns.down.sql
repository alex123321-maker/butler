DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM converter_developer_returns) THEN
  RAISE EXCEPTION 'cannot discard retained developer return history';
 END IF;
END $$;
DROP TABLE converter_developer_returns;

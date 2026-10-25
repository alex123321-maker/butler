DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM input_stopped_attempts) THEN
  RAISE EXCEPTION 'stopped attempts require their retained isolation guards';
 END IF;
END $$;
DROP TABLE input_stopped_attempts;

DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM voice_clips) OR EXISTS(SELECT 1 FROM voice_input_sources) THEN
  RAISE EXCEPTION 'voice audit/reference history must be retained';
 END IF;
END $$;
DROP TABLE voice_input_sources;
DROP TABLE voice_clips;

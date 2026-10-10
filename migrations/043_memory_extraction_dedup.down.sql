DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_extraction_dedup) OR EXISTS(SELECT 1 FROM memory_extraction_runs) THEN
  RAISE EXCEPTION 'cannot discard extraction deduplication history';
 END IF;
END $$;
DROP TABLE memory_extraction_dedup;
DROP TABLE memory_extraction_runs;

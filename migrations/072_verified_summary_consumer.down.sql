DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_summary_sources) THEN
  RAISE EXCEPTION 'retained summary source/deletion history prevents rollback';
 END IF;
END $$;
DROP TRIGGER invalidate_tool_summary ON tool_calls;
DROP FUNCTION butler_invalidate_tool_summary();
DROP TABLE memory_summary_sources;
ALTER TABLE session_summaries DROP COLUMN result_source_id;

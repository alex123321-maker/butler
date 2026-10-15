DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_summary_sources) THEN
  RAISE EXCEPTION 'retained summary source/deletion history prevents rollback';
 END IF;
END $$;
DROP TRIGGER invalidate_tool_summary ON tool_calls;
DROP FUNCTION butler_invalidate_tool_summary();
DROP TRIGGER summary_task_working_publication_fence ON tasks;
DROP TRIGGER summary_working_publication_fence ON memory_working;
DROP TRIGGER summary_message_publication_fence ON messages;
DROP TRIGGER summary_artifact_publication_fence ON artifacts;
DROP TRIGGER summary_final_publication_fence ON run_final_results;
DROP FUNCTION butler_memory_summary_publication_fence();
DROP FUNCTION butler_memory_summary_sources_current(TEXT[]);
DROP TABLE memory_summary_dependencies;
DROP TABLE memory_summary_sources;
ALTER TABLE session_summaries DROP COLUMN result_source_id;

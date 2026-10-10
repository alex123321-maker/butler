-- Privacy redaction cannot be undone by migration rollback. Preserve redaction
-- columns/guards and tombstones; downgrade requires a reviewed forward migration.
DROP TRIGGER IF EXISTS memory_register_message ON messages;
DROP TRIGGER IF EXISTS memory_final_publication_fence ON run_final_results;
DROP TRIGGER IF EXISTS memory_artifact_publication_fence ON artifacts;
DROP TRIGGER IF EXISTS memory_message_publication_fence ON messages;
DROP TRIGGER IF EXISTS memory_working_publication_fence ON memory_working;
DROP TRIGGER IF EXISTS memory_task_working_fence ON tasks;
DROP FUNCTION IF EXISTS butler_memory_task_working_fence();
DROP FUNCTION IF EXISTS butler_memory_register_message();
DROP FUNCTION IF EXISTS butler_memory_publication_fence();
DROP FUNCTION IF EXISTS butler_memory_check_run_sources(TEXT);
DROP TABLE IF EXISTS memory_run_transcripts;
DROP TABLE IF EXISTS memory_run_sources;

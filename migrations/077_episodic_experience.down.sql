DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_experience_observations) OR EXISTS(SELECT 1 FROM memory_experience_lessons) OR EXISTS(SELECT 1 FROM memory_experience_feedback) OR EXISTS(SELECT 1 FROM memory_experience_pins) THEN RAISE EXCEPTION 'retain experience provenance and feedback history'; END IF;
END $$;
DROP TRIGGER experience_final_publication_fence ON run_final_results;
DROP TRIGGER experience_artifact_publication_fence ON artifacts;
DROP TRIGGER experience_message_publication_fence ON messages;
DROP TRIGGER experience_working_publication_fence ON memory_working;
DROP TRIGGER experience_task_working_publication_fence ON tasks;
DROP FUNCTION butler_memory_experience_publication_fence();
DROP FUNCTION butler_memory_experience_sources_current(TEXT[]);
DROP TRIGGER experience_tool_head_update ON tool_calls;
DROP FUNCTION butler_memory_experience_tool_head_update();
DROP TABLE memory_experience_tool_heads;
DROP FUNCTION butler_memory_experience_tool_hash(tool_calls);
DROP FUNCTION butler_memory_experience_lessons_eligible(TEXT[],TEXT);
DROP TABLE memory_experience_pins,memory_experience_feedback,memory_experience_lessons,memory_experience_tool_pins,memory_experience_observations;
DROP FUNCTION butler_experience_metadata_immutable();

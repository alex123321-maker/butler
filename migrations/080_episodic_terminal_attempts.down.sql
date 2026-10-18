DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_experience_terminal_attempts) THEN
  RAISE EXCEPTION 'retain stopped-attempt provenance and currentness fences';
 END IF;
END $$;
CREATE OR REPLACE FUNCTION butler_memory_experience_sources_current(ids TEXT[]) RETURNS BOOLEAN STABLE LANGUAGE SQL AS $$
 WITH RECURSIVE roots AS(
  SELECT unnest(ids) AS id UNION SELECT d.parent_source_id FROM memory_source_dependencies d JOIN roots r ON r.id=d.child_source_id
 ),observations AS(SELECT o.* FROM memory_experience_observations o JOIN roots r ON r.id=o.source_id)
 SELECT NOT EXISTS(SELECT 1 FROM observations o LEFT JOIN runs r ON r.run_id=o.run_id WHERE r.session_key IS DISTINCT FROM o.session_key)
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_tool_pins p ON p.observation_source_id=o.source_id
 LEFT JOIN memory_experience_tool_heads h ON h.tool_call_id=p.tool_call_id
 WHERE h.run_id IS DISTINCT FROM p.run_id OR p.run_id<>o.run_id OR h.row_hash IS DISTINCT FROM p.row_hash
 OR h.revision IS DISTINCT FROM p.row_revision OR h.alive IS DISTINCT FROM TRUE)
 AND NOT EXISTS(SELECT 1 FROM observations o WHERE NOT EXISTS(SELECT 1 FROM memory_experience_tool_pins p WHERE p.observation_source_id=o.source_id))
$$;
DROP FUNCTION butler_memory_terminal_attempt_current(TEXT);
DROP FUNCTION butler_lock_memory_terminal_attempts(TEXT[]);
DROP TRIGGER experience_terminal_run_fence ON runs;
DROP FUNCTION butler_memory_terminal_run_fence();
DROP TABLE memory_experience_terminal_attempts;
DROP TRIGGER experience_input_head_update ON accepted_inputs;
DROP FUNCTION butler_memory_experience_input_head_update();
DROP TABLE memory_experience_input_heads;
DROP FUNCTION butler_memory_experience_input_hash(accepted_inputs);

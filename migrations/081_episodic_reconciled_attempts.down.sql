DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_experience_reconciled_attempts) OR EXISTS(SELECT 1 FROM memory_experience_terminal_actions) THEN
  RAISE EXCEPTION 'retain separately reconciled attempt provenance and currentness fences';
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
 AND NOT EXISTS(SELECT 1 FROM observations o JOIN memory_experience_terminal_attempts a USING(source_id) WHERE NOT butler_memory_terminal_attempt_current(o.source_id))
$$;
DROP FUNCTION butler_lock_memory_reconciled_attempts(TEXT[]);
DROP FUNCTION butler_memory_reconciled_attempt_current(TEXT);
DROP FUNCTION butler_memory_reconciled_context(TEXT,TEXT,TEXT,TEXT);
DROP TABLE memory_experience_terminal_actions;
DROP TABLE memory_experience_reconciled_pins;
DROP TABLE memory_experience_reconciled_attempts;
DROP TRIGGER experience_reconciled_run ON runs;
DROP TRIGGER experience_reconciled_control ON task_control_events;
DROP TRIGGER experience_reconciled_target ON task_control_targets;
DROP TRIGGER experience_reconciled_transition ON run_state_transitions;
DROP TRIGGER experience_reconciled_final ON run_final_results;
DROP TRIGGER experience_reconciled_effect ON action_effects;
DROP TRIGGER experience_reconciled_input ON accepted_inputs;
DROP FUNCTION butler_memory_reconciled_head_update();
DROP FUNCTION butler_memory_reconciled_projection(TEXT,JSONB);
DROP FUNCTION butler_memory_reconciled_row_id(TEXT,JSONB);
DROP TABLE memory_experience_reconciled_heads;

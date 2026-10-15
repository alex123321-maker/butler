-- Disabling a consumer or rolling back an image must retain an existing
-- privacy fence. Empty development schemas can still reverse this migration.
DO $$
DECLARE used BOOLEAN;
BEGIN
 IF to_regclass('memory_run_payload_redactions') IS NOT NULL THEN
  EXECUTE 'SELECT EXISTS(SELECT 1 FROM memory_run_payload_redactions)' INTO used;
  IF used THEN RAISE EXCEPTION 'cannot remove a durable run payload privacy fence'; END IF;
 END IF;
END $$;

DROP TRIGGER IF EXISTS experience_tool_publication_fence ON tool_calls;
DROP TRIGGER IF EXISTS temporal_tool_publication_fence ON tool_calls;
DROP TRIGGER IF EXISTS summary_tool_publication_fence ON tool_calls;
DROP TRIGGER IF EXISTS memory_tool_publication_fence ON tool_calls;
DROP TRIGGER IF EXISTS memory_approval_event_payload_fence ON approval_events;
DROP TRIGGER IF EXISTS memory_approval_payload_fence ON approvals;
DROP TRIGGER IF EXISTS memory_tool_payload_fence ON tool_calls;
DROP FUNCTION IF EXISTS butler_memory_tool_payload_fence();
DROP TABLE IF EXISTS memory_run_payload_redactions;
DROP FUNCTION IF EXISTS butler_memory_payload_redaction_identity();

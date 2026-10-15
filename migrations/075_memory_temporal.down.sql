DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_claim_temporal_revisions) OR EXISTS(SELECT 1 FROM memory_temporal_source_pins) OR EXISTS(SELECT 1 FROM memory_temporal_run_pins) OR EXISTS(SELECT 1 FROM memory_fact_decision_history) THEN
  RAISE EXCEPTION 'preserve temporal and owner decision history; downgrade refused';
 END IF;
END $$;
DROP TRIGGER memory_temporal_redaction ON memory_claims;
DROP FUNCTION butler_redact_memory_temporal();
DROP TRIGGER memory_fact_decision_record ON memory_fact_heads;
DROP FUNCTION butler_record_memory_fact_decision();
DROP TRIGGER temporal_final_publication_fence ON run_final_results;
DROP TRIGGER temporal_artifact_publication_fence ON artifacts;
DROP TRIGGER temporal_message_publication_fence ON messages;
DROP TRIGGER temporal_working_publication_fence ON memory_working;
DROP TRIGGER temporal_task_working_publication_fence ON tasks;
DROP FUNCTION butler_memory_temporal_publication_fence();
DROP FUNCTION butler_memory_temporal_run_current(TEXT);
DROP FUNCTION butler_memory_temporal_sources_current(TEXT[]);
DROP FUNCTION butler_memory_temporal_pins_current(JSONB);
DROP TABLE memory_temporal_run_pins;
DROP TABLE memory_temporal_source_pins;
DROP FUNCTION butler_memory_temporal_freshness(TEXT,TIMESTAMPTZ);
DROP FUNCTION butler_memory_temporal_metadata_hash(TEXT,TIMESTAMPTZ);
DROP TABLE memory_fact_decision_history;
DROP TABLE memory_claim_temporal_revisions;
DROP TABLE memory_claim_temporal_heads;
DROP FUNCTION butler_memory_temporal_immutable();

DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_claim_temporal_revisions) OR EXISTS(SELECT 1 FROM memory_temporal_source_pins) OR EXISTS(SELECT 1 FROM memory_fact_decision_history) THEN
  RAISE EXCEPTION 'preserve temporal and owner decision history; downgrade refused';
 END IF;
END $$;
DROP TRIGGER memory_temporal_redaction ON memory_claims;
DROP FUNCTION butler_redact_memory_temporal();
DROP TRIGGER memory_fact_decision_record ON memory_fact_heads;
DROP FUNCTION butler_record_memory_fact_decision();
DROP FUNCTION butler_memory_temporal_sources_current(TEXT[]);
DROP TABLE memory_temporal_source_pins;
DROP FUNCTION butler_memory_temporal_freshness(TEXT,TIMESTAMPTZ);
DROP FUNCTION butler_memory_temporal_metadata_hash(TEXT,TIMESTAMPTZ);
DROP TABLE memory_fact_decision_history;
DROP TABLE memory_claim_temporal_revisions;
DROP TABLE memory_claim_temporal_heads;
DROP FUNCTION butler_memory_temporal_immutable();

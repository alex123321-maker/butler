DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM memory_consolidation_inputs)
 OR EXISTS(SELECT 1 FROM memory_consolidation_generations)
 OR EXISTS(SELECT 1 FROM memory_consolidation_decisions)
 OR EXISTS(SELECT 1 FROM memory_aux_jobs WHERE context_manifest<>'[]'::jsonb)
 OR EXISTS(SELECT 1 FROM memory_model_policies WHERE policy->>'handler'='memory-consolidation-v1') THEN
  RAISE EXCEPTION 'consolidation history must be retained';
 END IF;
END $$;
DROP TABLE memory_consolidation_decisions,memory_consolidation_heads,memory_consolidation_generations,memory_consolidation_inputs;
ALTER TABLE memory_aux_jobs DROP CONSTRAINT memory_job_context_manifest;
ALTER TABLE memory_aux_jobs DROP COLUMN context_manifest;

-- Exact typed owner contexts are immutable job manifest data, never model input.
-- Nil legacy policy fields remain omitted; [] preserves existing local job rows.
ALTER TABLE memory_aux_jobs ADD COLUMN context_manifest JSONB NOT NULL DEFAULT '[]';
ALTER TABLE memory_aux_jobs ADD CONSTRAINT memory_job_context_manifest CHECK (
 jsonb_typeof(context_manifest)='array' AND jsonb_array_length(context_manifest)<=8
 AND (context_manifest='[]'::jsonb OR kind='model' AND purpose='memory-consolidation-v1' AND task_id='' AND original_run_id='')
);

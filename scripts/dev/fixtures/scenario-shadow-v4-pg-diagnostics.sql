BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object(
 'version','butler-cloud-v4-pg-diagnostics-v1',
 'attempts',(SELECT COALESCE(json_agg(json_build_object(
  'ordinal',a.ordinal,'context_id',a.context_id,'outcome',a.outcome,
  'input_tokens',a.input_tokens,'output_tokens',a.output_tokens,'total_tokens',a.total_tokens,
  'collector_stage',d.collector_stage,'diagnostic_code',d.diagnostic_code,'encoding_reason',d.encoding_reason,
  'final_bytes',d.final_bytes,'final_hash',d.final_hash,'raw_retained',d.raw_retained,'retention_ok',d.retention_ok,
  'model_version',d.model_version,'finish_reason',d.finish_reason,'provider_stage',d.provider_stage,
  'final_count',d.final_count,'terminal_count',d.terminal_count,'lifecycle_clean',d.lifecycle_clean
 ) ORDER BY a.ordinal),'[]'::json) FROM fixture_scenario_shadow_v4_attempts a LEFT JOIN fixture_scenario_shadow_v4_diagnostics d USING(ordinal)));
COMMIT;

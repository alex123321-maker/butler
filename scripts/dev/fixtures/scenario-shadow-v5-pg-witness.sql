BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object(
  'version','butler-cloud-v5-pg-witness-v1',
  'window',(SELECT json_build_object('native_baseline',283,'attempts',attempts,'baseline_hash',baseline_hash,'policy_hash',policy_hash,'expires_at',expires_at) FROM fixture_scenario_shadow_v5_limit WHERE singleton),
  'attempts',(SELECT COALESCE(json_agg(json_build_object(
    'ordinal',ordinal,'phase',phase,'case_id',case_id,'role',role,'context_id',context_id,'parent_context_id',parent_context_id,
    'input_hash',input_hash,'created_at',created_at,'outcome',outcome,
    'input_tokens',input_tokens,'output_tokens',output_tokens,'total_tokens',total_tokens,'output_hash',output_hash
  ) ORDER BY ordinal),'[]'::json) FROM fixture_scenario_shadow_v5_attempts));
COMMIT;

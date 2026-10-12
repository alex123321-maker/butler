-- Zero-inference prepare-only formatter fixture in its disposable database.
CREATE TABLE fixture_scenario_shadow_v3_limit AS SELECT TRUE AS singleton,251 AS attempts,repeat('0',64) AS baseline_hash,repeat('1',64) AS policy_hash,clock_timestamp()+interval '1 hour' AS expires_at;
CREATE TABLE fixture_scenario_shadow_v3_attempts AS
SELECT ordinal,'quality'::text AS phase,'probe-case-'||ordinal AS case_id,'single'::text AS role,'probe-context-'||ordinal AS context_id,NULL::text AS parent_context_id,repeat('2',64) AS input_hash,clock_timestamp() AS created_at,
  CASE ordinal WHEN 249 THEN 'completed' WHEN 250 THEN 'transport_error' ELSE 'cancelled' END AS outcome,
  CASE ordinal WHEN 249 THEN 0::bigint WHEN 250 THEN 10::bigint ELSE NULL::bigint END AS input_tokens,
  CASE ordinal WHEN 249 THEN 0::bigint WHEN 250 THEN 2::bigint ELSE NULL::bigint END AS output_tokens,
  CASE ordinal WHEN 249 THEN 0::bigint WHEN 250 THEN 15::bigint ELSE NULL::bigint END AS total_tokens,
  CASE ordinal WHEN 249 THEN repeat('3',64) ELSE NULL::text END AS output_hash
FROM generate_series(249,251) AS ordinal;

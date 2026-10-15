#!/usr/bin/env bash
# Content-free accounting for the one isolated native recovery window.
set -euo pipefail
[[ "${1:?isolated root}" == /srv/storage/butler/agents/issue82-20261015 ]] || exit 2
docker exec -i butler-issue82-postgres-1 psql -v ON_ERROR_STOP=1 -v task="${2:-}" -U butler -d memory_jobs_test -At <<'SQL'
SELECT jsonb_build_object(
 'observed_at',clock_timestamp(),
 'control',(SELECT state FROM memory_model_control),
 'background_attempts',(SELECT count(*) FROM memory_model_attempts),
 'foreground_attempts',(SELECT count(*) FROM execution_model_attempts),
 'reservations',(SELECT count(*) FROM execution_budget_reservations),
 'active_policies',(SELECT count(*) FROM memory_model_policies WHERE state<>'revoked' AND (policy->'job'->>'deadline')::timestamptz>clock_timestamp()),
 'foreign_policies',(SELECT count(*) FROM memory_model_policies WHERE state<>'revoked' AND (policy->'job'->>'deadline')::timestamptz>clock_timestamp() AND policy_id NOT IN ('issue82-native-v3','issue82-local-v3','issue82-completed-v3')),
 'active_jobs',(SELECT count(*) FROM memory_aux_jobs WHERE status IN ('pending','running')),
 'active_inputs',(SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running')),
 'active_runs',(SELECT count(*) FROM runs WHERE current_state NOT IN ('completed','failed','cancelled','timed_out')),
 'active_leases',(SELECT count(*) FROM session_ownership WHERE lease_id<>'' AND expires_at>clock_timestamp()),
 'old_attempts',(SELECT count(*) FROM memory_model_attempts WHERE operation_id IN ('operation_f9dcb25655d2ed03e04455304297efd2e92c5eedb06fad4a','operation_d0d1681a99c1c84ca8d2aff957443c55537f5ffca4988838')),
 'old_unknown_usage',(SELECT count(*) FROM memory_model_attempts WHERE operation_id IN ('operation_f9dcb25655d2ed03e04455304297efd2e92c5eedb06fad4a','operation_d0d1681a99c1c84ca8d2aff957443c55537f5ffca4988838') AND usage IS NULL),
 'old_attempts_digest',(SELECT md5(string_agg(to_jsonb(a)::text,',' ORDER BY operation_id)) FROM memory_model_attempts a WHERE operation_id IN ('operation_f9dcb25655d2ed03e04455304297efd2e92c5eedb06fad4a','operation_d0d1681a99c1c84ca8d2aff957443c55537f5ffca4988838')),
 'old_reservations_digest',(SELECT md5(string_agg(to_jsonb(r)::text,',' ORDER BY reservation_id)) FROM execution_budget_reservations r WHERE reservation_id IN ('0d02f272d1441b77cc1819a4946bd23a35738c9f72d6a6af4ed88e74d232b631','ee8f677413d193b1c190691d5ac82aad8b5ea8b3809be28f77adc5a91cd6afb2')),
 'attempts',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY created_at) FROM memory_model_attempts a),'[]'),
 'foreground',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY created_at) FROM execution_model_attempts a),'[]'),
 'scopes',COALESCE((SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s),'[]'),
 'root',(SELECT jsonb_build_object('used',used_json,'limits',limits_json) FROM execution_budget_scopes WHERE kind='task' AND subject_id=:'task'),
 'local_attempts',(SELECT count(*) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE j.policy_id='issue82-local-v3'),
 'publications',COALESCE((SELECT jsonb_agg(jsonb_build_object('policy_id',j.policy_id,'job_id',j.job_id,'job_status',j.status,'attempt_state',a.state,'reservation_id',r.reservation_id,'source_id',s.source_id,'source_status',s.status,'source_revision',s.head_revision,'usage',a.usage,'parent_id',d.parent_source_id,'parent_revision',d.parent_revision,'parent_current_revision',p.head_revision,'parent_status',p.status)) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) JOIN execution_budget_reservations r USING(reservation_id) JOIN memory_sources s ON s.source_id=a.result_source_id JOIN memory_source_dependencies d ON d.child_source_id=s.source_id JOIN memory_sources p ON p.source_id=d.parent_source_id WHERE j.policy_id IN ('issue82-native-v3','issue82-completed-v3')),'[]'));
SQL

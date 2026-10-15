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
 'foreign_policies',(SELECT count(*) FROM memory_model_policies WHERE state<>'revoked' AND (policy->'job'->>'deadline')::timestamptz>clock_timestamp() AND policy_id NOT IN ('issue82-native-v8','issue82-local-v8','issue82-completed-v8')),
 'active_jobs',(SELECT count(*) FROM memory_aux_jobs WHERE status IN ('pending','running')),
 'active_inputs',(SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running')),
 'active_runs',(SELECT count(*) FROM runs WHERE current_state NOT IN ('completed','failed','cancelled','timed_out')),
 'active_leases',(SELECT count(*) FROM session_ownership WHERE lease_id<>'' AND expires_at>clock_timestamp()),
 'old_attempts',(SELECT count(*) FROM memory_model_attempts WHERE operation_id IN ('operation_f9dcb25655d2ed03e04455304297efd2e92c5eedb06fad4a','operation_d0d1681a99c1c84ca8d2aff957443c55537f5ffca4988838','operation_f9da1d58b73ace1b49f72871964966743d9d0e94344a23e3','operation_94cdb79617bc20850448d9cc131be1d01f694bcaf163fbc2','operation_2b029ca6cfb7576571cff8b1675ee738cc1b44f348ffd478','operation_b3029e9d239b5e9b924fd0f372c9d46d4203277a333bbd31','operation_c35bc265542fddc206ee464e3de88c6c281ff21cf06c3e9d')),
 'old_unknown_usage',(SELECT count(*) FROM memory_model_attempts WHERE operation_id IN ('operation_f9dcb25655d2ed03e04455304297efd2e92c5eedb06fad4a','operation_d0d1681a99c1c84ca8d2aff957443c55537f5ffca4988838','operation_f9da1d58b73ace1b49f72871964966743d9d0e94344a23e3','operation_94cdb79617bc20850448d9cc131be1d01f694bcaf163fbc2','operation_2b029ca6cfb7576571cff8b1675ee738cc1b44f348ffd478','operation_b3029e9d239b5e9b924fd0f372c9d46d4203277a333bbd31','operation_c35bc265542fddc206ee464e3de88c6c281ff21cf06c3e9d') AND usage IS NULL),
 'old_attempts_digest',(SELECT md5(string_agg(to_jsonb(a)::text,',' ORDER BY operation_id)) FROM memory_model_attempts a WHERE operation_id IN ('operation_f9dcb25655d2ed03e04455304297efd2e92c5eedb06fad4a','operation_d0d1681a99c1c84ca8d2aff957443c55537f5ffca4988838','operation_f9da1d58b73ace1b49f72871964966743d9d0e94344a23e3','operation_94cdb79617bc20850448d9cc131be1d01f694bcaf163fbc2','operation_2b029ca6cfb7576571cff8b1675ee738cc1b44f348ffd478','operation_b3029e9d239b5e9b924fd0f372c9d46d4203277a333bbd31','operation_c35bc265542fddc206ee464e3de88c6c281ff21cf06c3e9d')),
 'old_reservations_digest',(SELECT md5(string_agg(to_jsonb(r)::text,',' ORDER BY reservation_id)) FROM execution_budget_reservations r WHERE reservation_id IN ('0d02f272d1441b77cc1819a4946bd23a35738c9f72d6a6af4ed88e74d232b631','ee8f677413d193b1c190691d5ac82aad8b5ea8b3809be28f77adc5a91cd6afb2','d1f91a5ddc8279d6722eca2094eb741ee3ea88d50b18015b934d882c50332cc1','53dde430f9412ad81552342e59296918f26363a1785652a4f9e1d6a5566c0c72','aa566b6cb26f9b26db05dc8ec33d2b08e39675747a193064ac400453472f325e','3a0c89e0b6414287040faa90642e8d49d6bc2df72c89acd2eb885675e402dfbe','3bbdade6bea5c7f58d848c92fb37b936d3cdb9287c027a9ada62a6203d4fdb8b')),
 'attempts',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY created_at) FROM memory_model_attempts a),'[]'),
 'foreground',COALESCE((SELECT jsonb_agg(to_jsonb(a) ORDER BY created_at) FROM execution_model_attempts a),'[]'),
 'scopes',COALESCE((SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s),'[]'),
 'root',(SELECT jsonb_build_object('used',used_json,'limits',limits_json) FROM execution_budget_scopes WHERE kind='task' AND subject_id=:'task'),
 'local_attempts',(SELECT count(*) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE j.policy_id='issue82-local-v8'),
 'publications',COALESCE((SELECT jsonb_agg(jsonb_build_object('policy_id',j.policy_id,'job_id',j.job_id,'job_status',j.status,'attempt_state',a.state,'reservation_id',r.reservation_id,'source_id',s.source_id,'source_status',s.status,'source_revision',s.head_revision,'usage',a.usage,'parent_id',d.parent_source_id,'parent_revision',d.parent_revision,'parent_current_revision',p.head_revision,'parent_status',p.status)) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) JOIN execution_budget_reservations r USING(reservation_id) JOIN memory_sources s ON s.source_id=a.result_source_id JOIN memory_source_dependencies d ON d.child_source_id=s.source_id JOIN memory_sources p ON p.source_id=d.parent_source_id WHERE j.policy_id IN ('issue82-native-v8','issue82-completed-v8')),'[]'));
SQL

#!/usr/bin/env bash
# Read-only metadata snapshot for one exact attempt; no args/results/error text.
set -euo pipefail
umask 077
set -o noclobber
event=${1:?browser-baseline event required}
out=${2:?new output file required}
[[ "$event" =~ ^browser-baseline-[a-z0-9-]{1,90}$ ]] || exit 2
docker exec -i butler-main-postgres-1 psql -XqAt -v ON_ERROR_STOP=1 -U butler -d butler -v ev="$event" > "$out" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object(
 'scope','native_metadata_only','observed_at',clock_timestamp(),
 'event',i.event_id,'input',i.input_id,'run',i.run_id,'status',i.status,
 'accepted_at',i.accepted_at,'updated_at',i.updated_at,
 'pending_approvals',(SELECT count(*) FROM approvals a WHERE a.run_id=i.run_id AND a.status='pending'),
 'tools',(SELECT json_agg(json_build_object('name',t.tool_name,'status',t.status,'started_at',t.started_at,'finished_at',t.finished_at,'has_error',t.error IS NOT NULL AND t.error<>'{}'::jsonb) ORDER BY t.started_at) FROM tool_calls t WHERE t.run_id=i.run_id),
 'publications',(SELECT json_agg(json_build_object('ref',p.publication_id,'purpose',p.purpose,'created_at',p.created_at) ORDER BY p.created_at) FROM communication_publications p WHERE p.run_id=i.run_id AND NOT p.privacy_redacted),
 'usage',(SELECT json_build_object('attempts',count(*),'input_tokens',sum(actual_input_tokens),'output_tokens',sum(actual_output_tokens),'unknown_usage_pairs',count(*) FILTER(WHERE actual_input_tokens IS NULL OR actual_output_tokens IS NULL),'first_attempt',min(created_at),'last_usage_observed',max(observed_at)) FROM execution_model_attempts m WHERE m.run_id=i.run_id),
 'model_profile',(SELECT json_agg(v) FROM (SELECT DISTINCT provider,model FROM execution_model_attempts WHERE run_id=i.run_id) v),
 'deliveries',(SELECT json_agg(json_build_object('channel',d.channel,'state',d.status,'updated_at',d.updated_at)) FROM delivery_outbox d JOIN communication_publications p ON p.publication_id=d.publication_id WHERE p.run_id=i.run_id),
 'task_success','UNVERIFIED','monetary_cost',NULL)
FROM accepted_inputs i WHERE i.event_id=:'ev';
COMMIT;
SQL
test -s "$out"
printf 'Exact-attempt metadata saved; no task-success inference.\n'

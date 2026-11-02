#!/usr/bin/env bash
# Derive timings from existing records only: no Input submission/model/tool replay.
set -euo pipefail
umask 077
set -o noclobber
event=${1:?exact browser-baseline event}
out=${2:?new output file}
[[ "$event" =~ ^browser-baseline-[a-z0-9-]{1,90}$ ]] || exit 2
test ! -e "$out"
docker exec -i butler-main-postgres-1 psql -XqAt -v ON_ERROR_STOP=1 -U butler -d butler -v ev="$event" > "$out" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
WITH selected AS (SELECT * FROM accepted_inputs WHERE event_id=:'ev'),
calls AS (
 SELECT t.*,row_number() OVER(ORDER BY t.started_at,t.tool_call_id) AS ordinal,
 lag(t.finished_at) OVER(ORDER BY t.started_at,t.tool_call_id) AS previous_end,
 CASE WHEN t.finished_at>=t.started_at THEN round(extract(epoch FROM t.finished_at-t.started_at)*1000,3) END AS duration_ms
 FROM tool_calls t JOIN selected i ON t.run_id=i.run_id
), states AS (
 SELECT s.*,lead(s.transitioned_at) OVER(ORDER BY s.id) AS next_at
 FROM run_state_transitions s JOIN selected i ON i.run_id=s.run_id
)
SELECT json_build_object(
 'schema',1,'scope','existing_record_timings_only','observed_at',clock_timestamp(),
 'event',i.event_id,'input',i.input_id,'run',i.run_id,'input_status',i.status,
 'error_code',CASE WHEN i.error_code ~ '^[a-z0-9_]{1,64}$' THEN i.error_code END,
 'accepted_at',i.accepted_at,
 'input_duration_ms',CASE WHEN i.status IN ('completed','failed') AND i.updated_at>=i.accepted_at THEN round(extract(epoch FROM i.updated_at-i.accepted_at)*1000,3) END,
 'tool_spans',COALESCE((SELECT json_agg(json_build_object(
   'ordinal',ordinal,'tool',CASE WHEN tool_name ~ '^(browser|tools|web|communication|credentials|http|memory)[.][a-z_]+$' THEN tool_name ELSE 'other_tool' END,
   'status',status,'started_at',started_at,'finished_at',finished_at,'duration_ms',duration_ms,
   'since_input_ms',round(extract(epoch FROM started_at-i.accepted_at)*1000,3),
   'gap_before_ms',CASE WHEN ordinal=1 THEN round(extract(epoch FROM started_at-i.accepted_at)*1000,3) WHEN started_at>=previous_end THEN round(extract(epoch FROM started_at-previous_end)*1000,3) END,
   'overlaps_previous',COALESCE(started_at<previous_end,false),
   'local_validation_without_dispatch',status='validation_rejected') ORDER BY ordinal) FROM calls),'[]'::json),
 'tool_aggregate',COALESCE((SELECT json_agg(x ORDER BY x.tool) FROM (
   SELECT CASE WHEN tool_name ~ '^(browser|tools|web|communication|credentials|http|memory)[.][a-z_]+$' THEN tool_name ELSE 'other_tool' END AS tool,
   count(*) AS calls,count(duration_ms) AS measured_calls,sum(duration_ms) AS sum_ms,min(duration_ms) AS min_ms,max(duration_ms) AS max_ms,
   round(percentile_cont(0.5) WITHIN GROUP(ORDER BY duration_ms)::numeric,3) AS median_ms
   FROM calls GROUP BY 1) x),'[]'::json),
 'model_attempt_spans',COALESCE((SELECT json_agg(json_build_object(
   'ordinal',a.ordinal,'reserved_at',a.created_at,'usage_observed_at',a.observed_at,
   'reservation_to_usage_ms',CASE WHEN a.observed_at>=a.created_at THEN round(extract(epoch FROM a.observed_at-a.created_at)*1000,3) END,
   'input_tokens',a.actual_input_tokens,'output_tokens',a.actual_output_tokens,
   'usage_known',a.actual_input_tokens IS NOT NULL AND a.actual_output_tokens IS NOT NULL) ORDER BY a.ordinal)
   FROM (SELECT m.*,row_number() OVER(ORDER BY m.created_at,m.operation_id) AS ordinal FROM execution_model_attempts m WHERE m.run_id=i.run_id) a),'[]'::json),
 'state_spans',COALESCE((SELECT json_agg(json_build_object('state',to_state,'started_at',transitioned_at,'next_recorded_transition_at',next_at,
   'duration_ms',CASE WHEN next_at>=transitioned_at THEN round(extract(epoch FROM next_at-transitioned_at)*1000,3) END) ORDER BY id) FROM states),'[]'::json),
 'publications',COALESCE((SELECT json_agg(json_build_object('purpose',p.purpose,'created_at',p.created_at,'since_input_ms',round(extract(epoch FROM p.created_at-i.accepted_at)*1000,3)) ORDER BY p.created_at) FROM communication_publications p WHERE p.run_id=i.run_id AND NOT p.privacy_redacted),'[]'::json),
 'deliveries',COALESCE((SELECT json_agg(json_build_object('channel',d.channel,'state',d.status,'updated_at',d.updated_at,'since_input_ms',round(extract(epoch FROM d.updated_at-i.accepted_at)*1000,3))) FROM delivery_outbox d JOIN communication_publications p ON p.publication_id=d.publication_id WHERE p.run_id=i.run_id),'[]'::json),
 'notes',json_build_array('Tool spans are recorded Core intervals, including Broker/runtime overhead; not isolated gesture/network times.',
   'Gaps include model, scheduling, context and other work; they are not pure model latency.',
   'Reservation-to-usage spans are accounting observations, not provider-only latency; NULL remains unknown.',
   'State spans overlap tool/accounting spans and must not be added as disjoint wall-time components.',
   'Telegram sent is a persisted API acknowledgement; Web result_ready is availability, neither proves a human read.'),
 'new_model_calls',0,'replayed_actions',0,'monetary_cost',NULL)
FROM selected i;
COMMIT;
SQL
jq -e -s 'length==1 and .[0].scope=="existing_record_timings_only"' "$out" >/dev/null
printf 'Existing-record timings saved; no action/model replay.\n'

#!/usr/bin/env bash
# Exact owner/session metadata only; do not export bindings, args or messages.
set -euo pipefail
umask 077
set -o noclobber
out=${1:?new private output file}
docker exec -i butler-main-postgres-1 psql -XqAt -v ON_ERROR_STOP=1 -U butler -d butler > "$out" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
WITH current_input AS (
 SELECT owner_id,session_key FROM accepted_inputs WHERE event_id='browser-baseline-matrix-20261028-nav-01'
), stops AS (
 SELECT f.* FROM input_stopped_attempts f JOIN current_input n USING(owner_id,session_key) WHERE f.classification='uncertain'
)
SELECT json_build_object('input',f.input_id,'run',f.run_id,'reason',f.reason,'created_at',f.created_at,
 'effects',(SELECT json_agg(json_build_object('id',e.action_id,'state',e.state,'tool',e.initial_binding->>'tool_name','effect',e.initial_binding->>'effect','verification',e.policy->>'verification_method','dispatch',e.dispatch_outcome,'evidence_count',(SELECT count(*) FROM action_effect_evidence v WHERE v.action_id=e.action_id)))
 FROM action_effects e WHERE e.initial_binding->>'run_id'=f.run_id OR e.action_id IN(SELECT effect_id FROM action_intents WHERE run_id=f.run_id)),
 'tools',(SELECT json_agg(json_build_object('id',t.tool_call_id,'name',t.tool_name,'status',t.status,'finished',t.finished_at IS NOT NULL)) FROM tool_calls t WHERE t.run_id=f.run_id),
 'publications',(SELECT json_agg(json_build_object('id',p.publication_id,'purpose',p.purpose)) FROM communication_publications p WHERE p.run_id=f.run_id),
 'deliveries',(SELECT json_agg(json_build_object('channel',d.channel,'status',d.status)) FROM delivery_outbox d WHERE d.run_id=f.run_id))
FROM stops f ORDER BY f.created_at;
COMMIT;
SQL
test -s "$out"

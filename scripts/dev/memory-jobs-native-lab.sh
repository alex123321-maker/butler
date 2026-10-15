#!/usr/bin/env bash
# One fresh bounded native window. Never rerun a consumed label or reset spend.
set -euo pipefail
base=${1:?isolated root}
revision=${2:?reviewed source commit}
[[ "$base" == /srv/storage/butler/agents/issue82-20261015 && "$revision" =~ ^[a-f0-9]{40}$ ]] || exit 2
export ISSUE82_ROOT=$base
label=${3:-native-v1}
prior=${4:-0}
[[ "$label" == native-v1 && "$prior" == 0 || "$label" == native-v2 && "$prior" == 1 ]] || exit 2
remaining=$((3-prior))
policy_id=issue82-$label
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir "$proof"
expires=$(date -u -d '+20 minutes' +%Y-%m-%dT%H:%M:%SZ)
jq -n --arg revision "$revision" --arg expires "$expires" --argjson remaining "$remaining" --argjson prior "$prior" '{revision:$revision,expires_at:$expires,provider:"antigravity",model:"gemini-3.8-flash-tiered",max_model_turns:$remaining,retained_prior_attempts:$prior,total_owner_day_model_turns:3,maximum_attempt_millis:90000,maximum_reserved_tokens:3170304,maximum_provider_input_bytes:25165824,cost_basis:"subscription_no_overage",monetary_overage_allowed:false,retry_or_fallback_outside_window:false,stop_on:["authority_failure","privacy_failure","accounting_failure","hard_deadline","finite_allowance"]}' > "$proof/caps.json"
sha256sum "$base/reports/orchestrator" "$base/reports/tool-broker" > "$proof/binaries.sha256"
compose=(docker compose -p butler-issue82 -f "$base/current/deploy/memory-jobs/compose.yml" -f "$base/current/deploy/memory-jobs/native.compose.yml")
# Enforce the prospective wall-clock stop on the entire isolated process, even
# if an API request or a foreground run is still waiting when the window ends.
(
 sleeper=''
 trap 'test -z "$sleeper" || kill "$sleeper" 2>/dev/null || true; exit 0' TERM
 sleep "$(( $(date -u -d "$expires" +%s) - $(date -u +%s) ))" &
 sleeper=$!
 wait "$sleeper"
 "${compose[@]}" stop --timeout 2 orchestrator > "$proof/deadline-stop.log" 2>&1
) &
watchdog=$!
# This file is a private credential reference; the token never enters tool args.
read -r owner_token < "$base/private/owner.token"
printf 'header = "Authorization: Bearer %s"\nheader = "Content-Type: application/json"\nheader = "X-Butler-Delivery-ID: issue82-native-v1"\n' "$owner_token" > "$base/private/curl.cfg"
unset owner_token
request(){ curl --config "$base/private/curl.cfg" --fail-with-body --silent --show-error --max-time 120 "$@"; }
stop(){
 kill "$watchdog" 2>/dev/null || true
 wait "$watchdog" 2>/dev/null || true
 request -d '{"state":"disabled"}' http://127.0.0.1:18282/api/v4/memory/model-worker-control > "$proof/stop.json" || true
 "${compose[@]}" stop --timeout 2 orchestrator > "$proof/service-stop.log" 2>&1 || true
}
trap stop EXIT
count=$(docker exec butler-issue82-postgres-1 psql -U butler -d memory_jobs_test -Atc 'SELECT (SELECT count(*) FROM execution_model_attempts)+(SELECT count(*) FROM memory_model_attempts)')
test "$count" = "$prior"
jq -n --arg id "$policy_id" '{external_key:$id,source_kind:"document",scope_type:"user",scope_id:"issue82-owner",title:"Bounded diagnostic source",body:"ISSUE82_BACKGROUND_SOURCE_20261015"}' > "$proof/source-request.json"
request -d @"$proof/source-request.json" http://127.0.0.1:18282/api/v4/memory/sources > "$proof/source.json"
jq --arg expires "$expires" --arg id "$policy_id" --argjson remaining "$remaining" '{policy_id:$id,handler:"source-quote-v1",trigger:"source_changed",interval_seconds:60,max_jobs:1,privacy:"cloud_allowed",job:{sources:[{source_id:.source_id,revision:.revision,access_revision:.access_revision,content_hash:.content_hash}],allowance:{model_turns:$remaining,tokens:(1056768*$remaining),cost_micros:0,active_millis:(90000*$remaining),provider_input_bytes:(8388608*$remaining)},deadline:$expires}}' "$proof/source.json" > "$proof/policy-request.json"
request -d @"$proof/policy-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/policy.json"
request -d '{"state":"enabled"}' http://127.0.0.1:18282/api/v4/memory/model-worker-control > "$proof/start.json"
for _ in $(seq 1 340); do
 request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/jobs.json"
 state=$(jq -r --arg id "$policy_id" '.policies[]|select(.policy_id==$id)|.jobs[0].status//"pending"' "$proof/jobs.json")
 if [[ "$state" == done ]];then break;fi
 if [[ "$state" == failed || "$state" == cancelled ]];then exit 3;fi
 [[ "$(date -u +%s)" -lt "$(date -u -d "$expires" +%s)" ]] || exit 4
 sleep 1
done
[[ "$state" == done ]]
result=$(jq -r --arg id "$policy_id" '.policies[]|select(.policy_id==$id)|.jobs[0].result_source_id' "$proof/jobs.json")
[[ "$result" =~ ^source_[a-f0-9]+$ ]]
request "http://127.0.0.1:18282/api/v4/memory/source?source_id=$result" > "$proof/result.json"
# Repeat event admission and restart the actual service; publication must survive
# and no paid operation may replay after losing the caller's acknowledgement.
request -d @"$proof/policy-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/repeat-policy.json"
"${compose[@]}" restart orchestrator > "$proof/restart.log" 2>&1
for _ in $(seq 1 30);do if curl -fsS http://127.0.0.1:18282/health > "$proof/health.json";then break;fi;sleep 1;done
request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/jobs-after-restart.json"
jq -e --arg result "$result" --arg id "$policy_id" '[.policies[]|select(.policy_id==$id)|.jobs[]]|length==1 and .[0].status=="done" and .[0].result_source_id==$result' "$proof/jobs-after-restart.json" > /dev/null

# An ordinary user run finishes while optional local-only memory is pending.
# No native retry is added by this script; all calls share the fixed owner/day cap.
jq --arg expires "$expires" '{policy_id:"issue82-local-wait",handler:"source-quote-v1",trigger:"source_changed",interval_seconds:60,max_jobs:1,privacy:"local_only",job:{sources:[{source_id:.source_id,revision:.revision,access_revision:.access_revision,content_hash:.content_hash}],allowance:{model_turns:1,tokens:1056768,cost_micros:0,active_millis:90000,provider_input_bytes:8388608},deadline:$expires}}' "$proof/source.json" > "$proof/local-request.json"
request -d @"$proof/local-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/local-policy.json"
for _ in $(seq 1 30);do
 request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/local-wait-before-input.json"
 if jq -e '.policies[]|select(.policy_id=="issue82-local-wait")|.jobs[0]|.status=="pending" and .reason=="local_backend_unavailable"' "$proof/local-wait-before-input.json" >/dev/null;then break;fi
 sleep 1
done
jq -e '.policies[]|select(.policy_id=="issue82-local-wait")|.jobs[0]|.status=="pending" and .reason=="local_backend_unavailable"' "$proof/local-wait-before-input.json" >/dev/null
[[ "$(( $(date -u +%s) + 300 ))" -lt "$(date -u -d "$expires" +%s)" ]] || exit 4
request -d '{"event_id":"issue82-foreground-v1","idempotency_key":"issue82-foreground-v1","event_type":"USER_MESSAGE","session_key":"web:issue82-owner","source":"web","payload":{"text":"Ответь ровно ISSUE82_FOREGROUND_READY. Инструменты не нужны.","user":"issue82-owner"}}' http://127.0.0.1:18282/api/v1/events > "$proof/input.json"
input=$(jq -r '.input_id' "$proof/input.json")
[[ "$input" =~ ^input-[a-f0-9]+$ ]]
for _ in $(seq 1 240);do
 request "http://127.0.0.1:18282/api/v3/inputs/$input" > "$proof/input-final.json"
 state=$(jq -r .status "$proof/input-final.json")
 if [[ "$state" == completed ]];then break;fi
 if [[ "$state" == failed || "$state" == blocked || "$state" == cancelled ]];then exit 5;fi
 sleep 1
done
[[ "$state" == completed ]]
request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/jobs-final.json"
jq -e '.policies[]|select(.policy_id=="issue82-local-wait")|.jobs[0]|.status=="pending"' "$proof/jobs-final.json" >/dev/null
curl -fsS http://127.0.0.1:18282/metrics > "$proof/metrics.txt"
docker exec butler-issue82-postgres-1 psql -U butler -d memory_jobs_test -Atc "SELECT jsonb_build_object('attempts',(SELECT jsonb_agg(jsonb_build_object('operation_id',operation_id,'job_id',job_id,'state',state,'usage',usage,'quote',quote,'result_source_id',result_source_id)) FROM memory_model_attempts),'foreground_attempts',(SELECT count(*) FROM execution_model_attempts),'scopes',(SELECT jsonb_agg(jsonb_build_object('kind',kind,'used',used_json,'limits',limits_json)) FROM execution_budget_scopes),'model_results',(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate'),'run_states',(SELECT jsonb_agg(jsonb_build_object('run_id',run_id,'state',current_state)) FROM runs))" > "$proof/ledger.json"
jq -e '.model_results==1 and .foreground_attempts>=1 and ([.attempts[]|select(.state=="published")]|length)==1 and ([.scopes[]|select(.kind=="owner_day")][0].used.model_turns<=3)' "$proof/ledger.json" >/dev/null
printf 'PASS\n' > "$proof/complete"
printf 'Native window complete: %s\n' "$proof"

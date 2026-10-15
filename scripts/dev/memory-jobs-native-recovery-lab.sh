#!/usr/bin/env bash
# Fresh native-v3 only. The earlier windows and unknown costs stay untouched.
set -euo pipefail
base=${1:?isolated root}
revision=${2:?frozen commit}
[[ "$base" == /srv/storage/butler/agents/issue82-20261015 && "$revision" =~ ^[a-f0-9]{40}$ ]] || exit 2
export ISSUE82_ROOT=$base
proof=$base/reports/native-v3
test ! -e "$proof"
umask 077
mkdir "$proof"
expires=$(date -u -d '+20 minutes' +%Y-%m-%dT%H:%M:%SZ)
jq -n --arg revision "$revision" --arg expires "$expires" '{revision:$revision,expires_at:$expires,provider:"antigravity",model:"gemini-3.8-flash-tiered",new_model_turns:3,retained_prior_attempts:2,cumulative_max_attempts:5,maximum_attempt_millis:90000,new_reserved_tokens:3170304,new_reserved_millis:270000,new_reserved_input_bytes:25165824,background_policies:2,allowance_per_background_policy:1,root_task_model_turns:2,root_shared_by_foreground_and_completed_background:true,subscription_no_overage:true,fallback:false,stop_on:["authority_failure","privacy_failure","accounting_failure","model_failure","deadline","finite_allowance"]}' > "$proof/caps.json"
sha256sum "$base/reports/orchestrator" "$base/reports/tool-broker" > "$proof/binaries.sha256"
compose=(docker compose -p butler-issue82 -f "$base/current/deploy/memory-jobs/compose.yml" -f "$base/current/deploy/memory-jobs/native.compose.yml")
(
 sleeper=''
 trap 'test -z "$sleeper" || kill "$sleeper" 2>/dev/null || true; exit 0' TERM
 sleep "$(( $(date -u -d "$expires" +%s) - $(date -u +%s) ))" & sleeper=$!
 wait "$sleeper"
 "${compose[@]}" stop --timeout 2 orchestrator > "$proof/deadline-stop.log" 2>&1
) &
watchdog=$!
request(){ curl --config "$base/private/curl.cfg" --fail-with-body --silent --show-error --max-time 30 "$@"; }
stop(){
 kill "$watchdog" 2>/dev/null || true
 wait "$watchdog" 2>/dev/null || true
 request -d '{"state":"disabled"}' http://127.0.0.1:18282/api/v4/memory/model-worker-control > "$proof/stop.json" || true
 "${compose[@]}" stop --timeout 2 orchestrator > "$proof/service-stop.log" 2>&1 || true
}
trap stop EXIT
bash "$base/current/scripts/dev/memory-jobs-native-ledger-lab.sh" "$base" > "$proof/ledger-before.json"
jq -e '.control=="disabled" and .background_attempts==2 and .foreground_attempts==0 and .reservations==2 and .old_attempts==2 and .old_unknown_usage==2 and .active_policies==0 and .active_jobs==0 and .active_inputs==0 and .active_runs==0 and .active_leases==0' "$proof/ledger-before.json" >/dev/null
request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/before.json"
request -d '{"external_key":"issue82-native-v3-source","source_kind":"document","scope_type":"user","scope_id":"issue82-owner","title":"Bounded diagnostic source","body":"ISSUE82_BACKGROUND_SOURCE_20261015"}' http://127.0.0.1:18282/api/v4/memory/sources > "$proof/source.json"
policy(){
 jq --arg expires "$expires" --arg id "$1" --arg privacy "$2" --arg trigger "$3" --arg task "$4" --arg run "$5" '{policy_id:$id,handler:"source-quote-v1",trigger:$trigger,interval_seconds:60,max_jobs:1,privacy:$privacy,job:{task_id:$task,original_run_id:$run,sources:[{source_id:.source_id,revision:.revision,access_revision:.access_revision,content_hash:.content_hash}],allowance:{model_turns:1,tokens:1056768,cost_micros:0,active_millis:90000,provider_input_bytes:8388608},deadline:$expires}}' "$proof/source.json"
}
wait_job(){
 for _ in $(seq 1 180);do
  request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/$1-jobs.json"
  state=$(jq -r --arg id "$1" '.policies[]|select(.policy_id==$id)|.jobs[0].status//"pending"' "$proof/$1-jobs.json")
  [[ "$state" != failed && "$state" != cancelled ]] || return 3
  if [[ "$state" == done ]];then return 0;fi
  [[ "$(date -u +%s)" -lt "$(date -u -d "$expires" +%s)" ]] || return 4
  sleep 1
 done
 return 4
}
policy issue82-native-v3 cloud_allowed source_changed '' '' > "$proof/policy-request.json"
request -d @"$proof/policy-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/policy.json"
bash "$base/current/scripts/dev/memory-jobs-native-ledger-lab.sh" "$base" > "$proof/ledger-admission.json"
jq -e '.foreign_policies==0 and .active_policies==1 and .active_jobs==0 and .active_inputs==0 and .active_runs==0 and .active_leases==0 and .reservations==2' "$proof/ledger-admission.json" >/dev/null
request -d '{"state":"enabled"}' http://127.0.0.1:18282/api/v4/memory/model-worker-control > "$proof/start.json"
wait_job issue82-native-v3
result=$(jq -r '.policies[]|select(.policy_id=="issue82-native-v3")|.jobs[0].result_source_id' "$proof/issue82-native-v3-jobs.json")
request "http://127.0.0.1:18282/api/v4/memory/source?source_id=$result" > "$proof/result.json"
request -d @"$proof/policy-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/repeated-policy.json"
"${compose[@]}" restart orchestrator > "$proof/restart.log" 2>&1
for _ in $(seq 1 30);do if curl -fsS http://127.0.0.1:18282/health > "$proof/health.json";then break;fi;sleep 1;done
request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/after-restart.json"
jq -e --arg id "$result" '.policies[]|select(.policy_id=="issue82-native-v3")|.jobs|length==1 and .[0].status=="done" and .[0].result_source_id==$id' "$proof/after-restart.json" >/dev/null
policy issue82-local-v3 local_only source_changed '' '' > "$proof/local-request.json"
request -d @"$proof/local-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/local.json"
for _ in $(seq 1 30);do
 request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/local-before-input.json"
 if jq -e '.policies[]|select(.policy_id=="issue82-local-v3")|.jobs[0]|.status=="pending" and .reason=="local_backend_unavailable"' "$proof/local-before-input.json" >/dev/null;then break;fi
 sleep 1
done
jq -e '.policies[]|select(.policy_id=="issue82-local-v3")|.jobs[0]|.status=="pending" and .reason=="local_backend_unavailable"' "$proof/local-before-input.json" >/dev/null
[[ "$(( $(date -u +%s) + 500 ))" -lt "$(date -u -d "$expires" +%s)" ]] || exit 4
# Provision only the synthetic session identity; task/run/lease and completion
# proceed through their real service surfaces.
docker exec -i butler-issue82-postgres-1 psql -v ON_ERROR_STOP=1 -U butler -d memory_jobs_test <<'SQL' > "$proof/session-fixture.log"
INSERT INTO sessions(session_key,user_id,channel) VALUES('web:issue82-owner','issue82-owner','web') ON CONFLICT DO NOTHING;
SQL
request -d '{"session_key":"web:issue82-owner","goal":"Return exactly ISSUE82_FOREGROUND_READY without tools","completion_criteria":["exact foreground marker"]}' http://127.0.0.1:18282/api/v3/tasks > "$proof/task-created.json"
task=$(jq -r .task.task_id "$proof/task-created.json")
jq -n --arg task "$task" '{event_id:"issue82-foreground-v3",idempotency_key:"issue82-foreground-v3",event_type:"USER_MESSAGE",session_key:"web:issue82-owner",source:"web",payload:{text:"Ответь ровно ISSUE82_FOREGROUND_READY. Инструменты не нужны.",user:"issue82-owner",task_id:$task}}' > "$proof/input-request.json"
request -d @"$proof/input-request.json" http://127.0.0.1:18282/api/v1/events > "$proof/input.json"
input=$(jq -r '.input_id' "$proof/input.json")
for _ in $(seq 1 180);do
 request "http://127.0.0.1:18282/api/v3/inputs/$input" > "$proof/input-final.json"
 state=$(jq -r .status "$proof/input-final.json")
 [[ "$state" != failed && "$state" != blocked && "$state" != cancelled ]] || exit 5
 if [[ "$state" == completed ]];then break;fi
 sleep 1
done
[[ "$state" == completed ]]
task=$(jq -r .task_id "$proof/input-final.json")
run=$(jq -r .run_id "$proof/input-final.json")
request "http://127.0.0.1:18282/api/v2/tasks/$task" > "$proof/foreground-result.json"
jq -e '.final_result.content=="ISSUE82_FOREGROUND_READY"' "$proof/foreground-result.json" >/dev/null
request "http://127.0.0.1:18282/api/v3/tasks/$task" > "$proof/task-before.json"
rev=$(jq -r .task.revision "$proof/task-before.json")
# Operator verification of the synthetic result uses the existing owner surface.
jq -n --arg run "$run" --argjson rev "$rev" '{task_revision:$rev,run_id:$run,summary:"Verified synthetic foreground marker",evidence_refs:[("run:"+$run)],verified_criteria:["exact foreground marker"],owner_confirmed:true}' > "$proof/result-confirm-request.json"
request -d @"$proof/result-confirm-request.json" "http://127.0.0.1:18282/api/v3/tasks/$task/results" > "$proof/verified-result.json"
verified=$(jq -r .result.result_id "$proof/verified-result.json")
jq -n --arg result "$verified" --argjson rev "$rev" '{expected_revision:$rev,status:"completed",result_id:$result}' > "$proof/task-complete-request.json"
request -d @"$proof/task-complete-request.json" "http://127.0.0.1:18282/api/v3/tasks/$task/status" > "$proof/task-complete.json"
policy issue82-completed-v3 cloud_allowed session_completed "$task" "$run" > "$proof/completed-request.json"
request -d @"$proof/completed-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/completed-policy.json"
wait_job issue82-completed-v3
completed_result=$(jq -r '.policies[]|select(.policy_id=="issue82-completed-v3")|.jobs[0].result_source_id' "$proof/issue82-completed-v3-jobs.json")
request "http://127.0.0.1:18282/api/v4/memory/source?source_id=$completed_result" > "$proof/completed-result.json"
request "http://127.0.0.1:18282/api/v3/tasks/$task" > "$proof/task-final.json"
jq -e '.task.status=="completed"' "$proof/task-final.json" >/dev/null
request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/jobs-final.json"
jq -e '.policies[]|select(.policy_id=="issue82-local-v3")|.jobs[0]|.status=="pending"' "$proof/jobs-final.json" >/dev/null
curl -fsS http://127.0.0.1:18282/metrics > "$proof/metrics.txt"
bash "$base/current/scripts/dev/memory-jobs-native-ledger-lab.sh" "$base" "$task" > "$proof/ledger-final.json"
jq -e --slurpfile before "$proof/ledger-before.json" '
 .background_attempts==4 and .foreground_attempts==1 and .reservations==5 and
 .old_attempts==2 and .old_unknown_usage==2 and
 .old_attempts_digest==$before[0].old_attempts_digest and
 .old_reservations_digest==$before[0].old_reservations_digest and
 .foreign_policies==0 and .active_inputs==0 and .active_runs==0 and .active_leases==0 and
 .local_attempts==0 and .root.used.model_turns==2 and .root.limits.model_turns==2 and
 (.foreground|length==1 and .[0].actual_input_tokens!=null and .[0].actual_output_tokens!=null) and
 (.publications|length==2 and (map(.source_id)|unique|length)==2 and
  all(.job_status=="done" and .attempt_state=="published" and .source_status=="active" and
      .usage!=null and .parent_status=="active" and .parent_revision==.parent_current_revision)) and
 ([.scopes[]|select(.kind=="owner_day" and .owner_id=="issue82-owner")]|length==1 and
  .[0].used_json.model_turns==5 and .[0].used_json.tokens==5283840 and
  .[0].used_json.active_millis==450000 and .[0].used_json.provider_input_bytes==41943040)
' "$proof/ledger-final.json" >/dev/null
printf 'PASS\n' > "$proof/complete"

#!/usr/bin/env bash
# Continue only the two unspent calls inside the original native-v10 deadline.
set -euo pipefail
base=${1:?isolated root}
[[ "$base" == /srv/storage/butler/agents/issue82-20261015 ]] || exit 2
export ISSUE82_ROOT=$base
original=$base/reports/native-v10
proof=$base/reports/native-v10-remainder
test ! -e "$proof"
umask 077
mkdir "$proof"
expires=$(jq -r .expires_at "$original/caps.json")
[[ "$(( $(date -u +%s) + 300 ))" -lt "$(date -u -d "$expires" +%s)" ]]
jq --arg expires "$expires" '{original_revision:.revision,original_expires_at:$expires,remaining_model_turns:2,cumulative_max_attempts:12,prefix_published_attempts:1,provider:.provider,model:.model,network_route:.network_route,no_window_extension:true,no_refund:true}' "$original/caps.json" > "$proof/caps.json"
cp "$original/source.json" "$proof/source.json"
cp "$base/reports/broker-ready-after.json" "$proof/broker-ready.json"
jq -e '.list_tools_succeeded and .inference_calls==0 and .tool_executions==0' "$proof/broker-ready.json" >/dev/null
sed '/^[Hh]eader.*X-Butler-Delivery-ID:/d' "$base/private/curl.cfg" > "$base/private/recovery-curl.cfg"
request(){ curl --config "$base/private/recovery-curl.cfg" -H 'X-Butler-Delivery-ID: issue82-native-v10-remainder' --fail-with-body --silent --show-error --max-time 30 "$@"; }
compose=(docker compose -p butler-issue82 -f "$base/current/deploy/memory-jobs/compose.yml" -f "$base/current/deploy/memory-jobs/native.compose.yml")
(
 sleeper=''
 trap 'test -z "$sleeper" || kill "$sleeper" 2>/dev/null || true; exit 0' TERM
 sleep "$(( $(date -u -d "$expires" +%s) - $(date -u +%s) ))" & sleeper=$!
 wait "$sleeper"
 "${compose[@]}" stop --timeout 2 orchestrator > "$proof/deadline-stop.log" 2>&1
) &
watchdog=$!
stop(){
 kill "$watchdog" 2>/dev/null || true
 wait "$watchdog" 2>/dev/null || true
 request -d '{"state":"disabled"}' http://127.0.0.1:18282/api/v4/memory/model-worker-control > "$proof/stop.json" || true
 "${compose[@]}" stop --timeout 2 orchestrator > "$proof/service-stop.log" 2>&1 || true
}
trap stop EXIT
bash "$base/current/scripts/dev/memory-jobs-native-ledger-lab.sh" "$base" > "$proof/ledger-before.json"
jq -e --slurpfile old "$original/ledger-before.json" '
 .control=="disabled" and .background_attempts==10 and .foreground_attempts==0 and .reservations==10 and .active_inputs==0 and .active_leases==0 and .foreign_policies==0 and
 .old_attempts_digest==$old[0].old_attempts_digest and .old_reservations_digest==$old[0].old_reservations_digest and
 ([.attempts[]|select(.state=="published")]|length==1 and .[0].usage!=null)
' "$proof/ledger-before.json" >/dev/null
task=$(jq -r .task.task_id "$original/task-created.json")
request "http://127.0.0.1:18282/api/v3/tasks/$task" > "$proof/task-recovery-before.json"
jq -e '.task.status=="blocked"' "$proof/task-recovery-before.json" >/dev/null
rev=$(jq -r .task.revision "$proof/task-recovery-before.json")
jq -n --argjson rev "$rev" '{expected_revision:$rev,kind:"resume",instruction:"Ответь ровно ISSUE82_FOREGROUND_READY. Инструменты не нужны."}' > "$proof/resume-request.json"
curl --config "$base/private/recovery-curl.cfg" -H 'X-Butler-Delivery-ID: issue82-native-v10-owner-resume' --fail-with-body --silent --show-error --max-time 30 -d @"$proof/resume-request.json" "http://127.0.0.1:18282/api/v3/tasks/$task/control" > "$proof/resume.json"
input=$(jq -r .continuation_input_id "$proof/resume.json")
[[ "$input" != null && -n "$input" ]]
request -d '{"state":"enabled"}' http://127.0.0.1:18282/api/v4/memory/model-worker-control > "$proof/start.json"

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
policy issue82-completed-v10 cloud_allowed session_completed "$task" "$run" > "$proof/completed-request.json"
request -d @"$proof/completed-request.json" http://127.0.0.1:18282/api/v4/memory/model-policies > "$proof/completed-policy.json"
wait_job issue82-completed-v10
completed_result=$(jq -r '.policies[]|select(.policy_id=="issue82-completed-v10")|.jobs[0].result_source_id' "$proof/issue82-completed-v10-jobs.json")
request "http://127.0.0.1:18282/api/v4/memory/source?source_id=$completed_result" > "$proof/completed-result.json"
request "http://127.0.0.1:18282/api/v3/tasks/$task" > "$proof/task-final.json"
jq -e '.task.status=="completed"' "$proof/task-final.json" >/dev/null
request http://127.0.0.1:18282/api/v4/memory/model-jobs > "$proof/jobs-final.json"
jq -e '.policies[]|select(.policy_id=="issue82-local-v10")|.jobs[0]|.status=="pending"' "$proof/jobs-final.json" >/dev/null
curl -fsS http://127.0.0.1:18282/metrics > "$proof/metrics.txt"
bash "$base/current/scripts/dev/memory-jobs-native-ledger-lab.sh" "$base" "$task" > "$proof/ledger-final.json"
jq -e --slurpfile before "$proof/ledger-before.json" '
 .background_attempts==11 and .foreground_attempts==1 and .reservations==12 and
 .old_attempts==9 and .old_unknown_usage==9 and
 .old_attempts_digest==$before[0].old_attempts_digest and
 .old_reservations_digest==$before[0].old_reservations_digest and
 .foreign_policies==0 and .active_inputs==0 and .active_runs==0 and .active_leases==0 and
 .local_attempts==0 and .root.used.model_turns==2 and .root.limits.model_turns==2 and
 (.foreground|length==1 and .[0].actual_input_tokens!=null and .[0].actual_output_tokens!=null) and
 (.publications|length==2 and (map(.source_id)|unique|length)==2 and
  all(.job_status=="done" and .attempt_state=="published" and .source_status=="active" and
      .usage!=null and .parent_status=="active" and .parent_revision==.parent_current_revision)) and
 ([.scopes[]|select(.kind=="owner_day" and .owner_id=="issue82-owner")]|length==1 and
  .[0].used_json.model_turns==12 and .[0].used_json.tokens==12681216 and
  .[0].used_json.active_millis==1080000 and .[0].used_json.provider_input_bytes==100663296)
' "$proof/ledger-final.json" >/dev/null
printf 'PASS\n' > "$proof/complete"

#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cd "$base"
test ! -e public-outcome-v1
mkdir public-outcome-v1
root=review-snapshot-v1/actual
jq '{maximum_entries,execution_slots,active_slot,capture_error,started_at,finished_at,elapsed_millis,slots}' "$root/window-final.json" > public-outcome-v1/slot-outcomes.json
jq '{sequence,slot,kind,finished,usage,failure,context_end,milliseconds,provider_stages}' "$root/entry-025-end.json" > public-outcome-v1/last-native-entry.json
jq '{failed_runs:[.runs[]|select(.run_id=="run-1793187137559083680-1540bb2235d553ed")|{run_id,current_state,error_type,error_message,started_at,finished_at}],skill_tools:[.tool_calls[]|select(.run_id=="run-1793187137559083680-1540bb2235d553ed")|{tool_name,status,error,result_status:.result.status,http_status:.result.status_code,opened_items:(if .tool_name=="memory.open" then (.result.items|length) else null end)}]}' "$root/source-database.json" > public-outcome-v1/failure-diagnostic.json
cp bound-native-proxy-v2-exit.json bound-native-proxy-v2-started.txt bound-native-proxy-v2-finished.txt private-native-seals-v1.sha256 review-build-v1.log review-build-v1-exit.json review-run-v1.log review-run-v1-exit.json public-outcome-v1/
cp operator-native-proxy-v2/server-handback.json operator-native-proxy-v2/process-exit.json review-output-v1/cost-review.json public-outcome-v1/
stat -c '%y %n' "$root/entry-001-start.json" "$root/entry-025-end.json" > public-outcome-v1/entry-file-times.txt
find review-snapshot-v1 -type f | wc -l > public-outcome-v1/private-sealed-file-count.txt
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
test "$(d inspect -f '{{.Id}}' issue115-proxy-relay-20261028-v2)" = "$(cat proxy-relay-v2.cid)"
d stop --time 3 issue115-proxy-relay-20261028-v2 > public-outcome-v1/relay-stopped.txt
d logs issue115-proxy-relay-20261028-v2 > public-outcome-v1/relay-safe-log.txt 2>&1
printf '{"generation_requests_after_failed_window":0,"old_relay_stopped":true,"native_replay":false,"working_deployment_changed":false}\n' > public-outcome-v1/retention.json
cat public-outcome-v1/failure-diagnostic.json

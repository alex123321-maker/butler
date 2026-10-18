#!/usr/bin/env bash
# One exact reviewed #90 cohort. The shared auth lock spans fresh clone,
# all57 inputs, exact CAS, control-plane main discovery and a fresh snapshot.
set -euo pipefail
base=${1:?isolated issue90 root}
label=${2:?prepared native label}
plan_hash=${3:?reviewed exact native preseal sha256}
pins_hash=${4:?reviewed exact prepared-pins sha256}
auth_binary=${5:?existing reviewed provider-auth diagnostic binary}
auth_hash=${6:?reviewed diagnostic executable sha256}
[[ "$base" =~ ^/srv/storage/butler/agents/issue90-[A-Za-z0-9_-]+$ && "$label" =~ ^[a-z0-9-]+$ && "$plan_hash" =~ ^[a-f0-9]{64}$ && "$pins_hash" =~ ^[a-f0-9]{64}$ && "$auth_hash" =~ ^[a-f0-9]{64}$ && "$auth_binary" == /srv/storage/butler/agents/*/reports/provider-auth-current ]] || exit 2
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?exact retained memory_read_test DSN required}"
proof=$base/reports/$label
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
test "$(cat "$proof/source-reference")" = "$src"
test "$(cat "$proof/prepared")" = PASS
test "$(sha256sum "$proof/plan/preseal.json" | cut -d' ' -f1)" = "$plan_hash"
test "$(sha256sum "$proof/prepared-pins.sha256" | cut -d' ' -f1)" = "$pins_hash"
test "$(sha256sum "$auth_binary" | cut -d' ' -f1)" = "$auth_hash"
sha256sum --check --status "$proof/prepared-pins.sha256"
sha256sum --check --status "$proof/source.sha256"
sha256sum --check --status "$proof/history.sha256"
sha256sum --check --status "$proof/binaries.sha256"
test ! -e "$proof/auth-stage-begun.json"
test ! -e "$proof/actual"
test ! -e "$base/private/$label"
test "$(stat -c '%a' "$base/private")" = 700
test "$(stat -c '%u' "$base/private")" = "$(id -u)"
history_hash=$(sha256sum "$proof/history/history.json" | cut -d' ' -f1)
history_script=$(cat "$proof/history-checker")
case "$history_script" in memory-closed-temporal-v7-history.py|memory-closed-temporal-v8-history.py|memory-closed-temporal-v9-history.py) ;; *) exit 2 ;; esac
umask 077
exec 9>/srv/storage/butler/agents/native-provider-auth.lock
flock -n 9
python3 "$src/scripts/dev/$history_script" check-current "$proof/history/history.json" "$history_hash" > "$proof/pre-auth-current-ledgers.json"
docker run --rm --pull never --network none --user "$(id -u):$(id -g)" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$proof:/proof:ro" -v "$proof/bin:/artifacts:ro" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_BROKER_E2E_BINARY=/artifacts/tool-broker-experience -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker-experience.test \
 -e BUTLER_EXTENSION_E2E_BINARY=/artifacts/extension-experience.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/artifacts/http-experience.test \
 -e BUTLER_EXPERIENCE_NATIVE_HISTORY=/proof/history/history.json -e BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256="$history_hash" \
 -e BUTLER_EXPERIENCE_NATIVE_CHECK=1 -e BUTLER_EXPERIENCE_NATIVE_PLAN=/proof/plan/preseal.json -e BUTLER_EXPERIENCE_NATIVE_PLAN_SHA256="$plan_hash" \
 golang:1.27.2-bookworm /artifacts/app.test -test.v -test.timeout=3m -test.run '^TestNativeExperienceCheckPreparedWholeComparison$' > "$proof/pre-auth-pins-check.txt" 2>&1
private=$base/private/$label
python3 "$src/scripts/dev/memory-consolidation-auth.py" clone "$base" "$label" > "$proof/clone-safe.json"
handoff(){
 status=$?
 trap - EXIT
 set +e
 handback=0
 # Always read the final durable opaque value after process exit. A collector
 # snapshot made during cancellation must never race an upstream auth refresh.
 python3 "$src/scripts/dev/memory-experience-auth-recovery.py" "$base" "$label" "$plan_hash" > "$proof/recovery-safe.json" || handback=1
 if [[ "$handback" == 0 ]];then
  python3 "$src/scripts/dev/memory-consolidation-auth.py" return "$base" "$label" > "$proof/return-safe.json" || handback=1
 fi
 if [[ "$handback" == 0 ]];then
  docker run --rm --pull never --network container:butler-lab-orchestrator-1 --env-file "$private/main-diagnose.env" -v "$auth_binary:/provider-auth:ro" golang:1.27.2-bookworm /provider-auth -action diagnose-cloudcode > "$private/main-diagnosis.json" || handback=1
  jq -e -f "$src/scripts/dev/memory-consolidation-main-auth.jq" "$private/main-diagnosis.json" >/dev/null || handback=1
  if [[ "$handback" == 0 ]];then
   jq '{resolved,unsafe_refresh,expired:.after.expired,trace}' "$private/main-diagnosis.json" > "$proof/main-auth-safe.json"
   python3 "$src/scripts/dev/memory-consolidation-auth.py" snapshot "$base" "$label" > "$proof/main-snapshot-safe.json" || handback=1
  fi
 fi
 printf '%s\n' "$status" > "$proof/native-status"
 printf '%s\n' "$handback" > "$proof/handback-status"
 python3 "$src/scripts/dev/memory-experience-native-close.py" "$base" "$label" "$plan_hash" || handback=1
 [[ "$handback" == 0 ]] || exit 1
 exit "$status"
}
trap handoff EXIT
# Maximum supported whole bound is38h20, also clamped inside the collector to
# its sealed UTC owner/day deadline. The outer timeout adds only cleanup time;
# it cannot extend task authority, provider partitions or the640-entry journal.
docker run --rm --pull never --name "butler-issue90-$label-native" --network host --user "$(id -u):$(id -g)" --cpus 1 --memory 2g --pids-limit 256 --security-opt no-new-privileges \
 --env-file "$private/native.env" -v "$src:/src:ro" -v "$proof:/proof" -v "$proof/bin:/artifacts:ro" -v "$private:/native-private" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_EXPERIENCE_NATIVE_EXECUTE=1 \
 -e BUTLER_EXPERIENCE_NATIVE_PLAN=/proof/plan/preseal.json -e BUTLER_EXPERIENCE_NATIVE_PLAN_SHA256="$plan_hash" -e BUTLER_EXPERIENCE_NATIVE_APPROVED_SHA256="$plan_hash" \
 -e BUTLER_EXPERIENCE_NATIVE_HISTORY=/proof/history/history.json -e BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256="$history_hash" \
 -e BUTLER_EXPERIENCE_NATIVE_OUTPUT=/proof/actual -e BUTLER_EXPERIENCE_PROCESS_PROOF=/proof/actual/processes \
 -e BUTLER_EXPERIENCE_NATIVE_AUTH_VALUE_FILE=/native-private/auth-import.value -e BUTLER_EXPERIENCE_NATIVE_AUTH_RETURN_FILE=/native-private/auth-return.value \
 -e BUTLER_BROKER_E2E_BINARY=/artifacts/tool-broker-experience -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker-experience.test \
 -e BUTLER_EXTENSION_E2E_BINARY=/artifacts/extension-experience.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/artifacts/http-experience.test \
 -e HTTP_PROXY=http://127.0.0.1:18080 -e HTTPS_PROXY=http://127.0.0.1:18080 -e ALL_PROXY=http://127.0.0.1:18080 -e NO_PROXY=localhost,127.0.0.1,::1 \
 golang:1.27.2-bookworm /artifacts/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /artifacts/app.test -test.v=test2json -test.timeout=2310m -test.run '^TestNativeExperienceWholeComparison$' > "$proof/runner.jsonl" 2> "$proof/runner.log"

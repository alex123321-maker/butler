#!/usr/bin/env bash
# Execute only the independently reviewed v6 seal. The shared lock spans fresh
# main clone, all entries, exact CAS handback and actual main paid discovery.
set -euo pipefail
base=${1:?isolated issue88 root}
label=${2:?prepared native label}
pins_hash=${3:?reviewed exact prepared-pins sha256}
auth_binary=${4:?reviewed existing provider-auth diagnostic binary}
[[ "$base" =~ ^/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+$ && "$label" =~ ^[A-Za-z0-9_-]+$ && "$pins_hash" =~ ^[a-f0-9]{64}$ && "$auth_binary" == /srv/storage/butler/agents/*/reports/provider-auth-current ]] || exit 2
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?exact retained issue85_native_test PostgreSQL required}"
proof=$base/reports/$label
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
test -f "$proof/prepared"
read -r prepared_src < "$proof/source-reference"
test "$src" = "$prepared_src"
test "$(sha256sum "$proof/prepared-pins.sha256" | cut -d' ' -f1)" = "$pins_hash"
sha256sum --check --status "$proof/prepared-pins.sha256"
sha256sum --check --status "$proof/source.sha256"
test ! -e "$proof/auth-stage-begun.json"
test ! -e "$proof/scheduled-before-setup.json"
# Staging is checked before the auth helper can read main's encrypted record.
test -d "$base/private"
test "$(stat -c %a "$base/private")" = 700
test ! -e "$base/private/$label"
history_hash=$(sha256sum "$proof/history/history.json" | cut -d' ' -f1)
history_script=memory-temporal-successor-history.py
v6=0;v7=0;v8=0
case "$(jq -er .format "$proof/history/history.json")" in
 butler.temporal.native-history.v2) v6=1 ;;
 butler.temporal.native-history.v3) history_script=memory-temporal-v7-history.py;v7=1 ;;
 butler.temporal.native-history.v4) history_script=memory-temporal-v8-history.py;v8=1 ;;
 *) exit 2 ;;
esac
umask 077
exec 9>/srv/storage/butler/agents/native-provider-auth.lock
flock -n 9
python3 "$src/scripts/dev/$history_script" check-current "$proof/history/history.json" "$history_hash" > "$proof/pre-auth-current-ledgers.json"
docker run --rm --pull never --network none --user "$(id -u):$(id -g)" --cpus 1 --memory 2g --pids-limit 128 \
 -v "$src:/src:ro" -v "$proof:/proof:ro" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_TEMPORAL_CHECK_PINS=1 -e BUTLER_TEMPORAL_NATIVE_PROOF=/proof -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/proof/broker.test \
 golang:1.27.2-bookworm /proof/collector.test -test.v -test.run '^TestTemporalNativeCheckPreparedPins$' > "$proof/pre-auth-pins-check.txt" 2>&1
python3 "$src/scripts/dev/memory-consolidation-auth.py" clone "$base" "$label" > "$proof/clone-safe.json"
private=$base/private/$label
handoff(){
 status=$?
 trap - EXIT
 if [[ ! -e "$private/auth-return.value" ]];then
  python3 "$src/scripts/dev/memory-temporal-precollector-return.py" "$base" "$label" || exit 1
 fi
 python3 "$src/scripts/dev/memory-consolidation-auth.py" return "$base" "$label" > "$proof/return-safe.json" || exit 1
 docker run --rm --pull never --network container:butler-lab-orchestrator-1 --env-file "$private/main-diagnose.env" -v "$auth_binary:/provider-auth:ro" golang:1.27.2-bookworm /provider-auth -action diagnose-cloudcode > "$private/main-diagnosis.json" || exit 1
 jq -e -f "$src/scripts/dev/memory-consolidation-main-auth.jq" "$private/main-diagnosis.json" >/dev/null || exit 1
 jq '{resolved,unsafe_refresh,expired:.after.expired,trace}' "$private/main-diagnosis.json" > "$proof/main-auth-safe.json"
 python3 "$src/scripts/dev/memory-consolidation-auth.py" snapshot "$base" "$label" > "$proof/main-snapshot-safe.json" || exit 1
 printf '%s\n' "$status" > "$proof/native-status"
 exit "$status"
}
trap handoff EXIT
docker run --rm --pull never --network host --user "$(id -u):$(id -g)" --cpus 2 --memory 3g --pids-limit 256 --security-opt no-new-privileges \
 --env-file "$private/native.env" -v "$src:/src:ro" -v "$proof:/proof" -v "$private:/native-private" -w /src/apps/orchestrator/internal/app \
 -e GOTOOLCHAIN=local -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_TEMPORAL_NATIVE_V6="$v6" -e BUTLER_TEMPORAL_NATIVE_V7="$v7" -e BUTLER_TEMPORAL_NATIVE_V8="$v8" -e BUTLER_TEMPORAL_NATIVE_PROOF=/proof \
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/proof/broker.test \
 -e HTTP_PROXY=http://127.0.0.1:18080 -e HTTPS_PROXY=http://127.0.0.1:18080 -e ALL_PROXY=http://127.0.0.1:18080 -e NO_PROXY=localhost,127.0.0.1,::1 \
 golang:1.27.2-bookworm /proof/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /proof/collector.test -test.v -test.timeout=975m -test.run '^TestTemporalNativeExplicitLookup$' > "$proof/runner.jsonl" 2>&1

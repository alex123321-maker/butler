#!/usr/bin/env bash
# Only execute after independent review of the exact prepared-pins sha256.
# This holds the global lock over fresh clone, all calls, CAS and main diagnosis.
set -euo pipefail
base=${1:?isolated issue89 root}
label=${2:?prepared prospective native label}
pins_hash=${3:?operator-reviewed prepared-pins sha256}
auth_binary=${4:?existing reviewed provider-auth diagnostic binary}
[[ "$base" =~ ^/srv/storage/butler/agents/issue89-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ && "$pins_hash" =~ ^[a-f0-9]{64}$ && "$auth_binary" == /srv/storage/butler/agents/*/reports/provider-auth-current ]] || exit 2
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?isolated persistent *_test PostgreSQL required}"
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
test ! -e "$proof/screen/window-begun.json"
read -r history < "$proof/history-reference"
read -r history_hash < "$proof/history.sha256"
test "$(sha256sum "$history/history.json" | cut -d' ' -f1)" = "$history_hash"
history_format=$(jq -er .format "$history/history.json")
v1=0
v2=0
v3=0
case "$history_format" in
 butler.consolidation.native-history.v2) v1=1; history_script=memory-consolidation-history.py ;;
 butler.consolidation.native-history.v3) v2=1; history_script=memory-consolidation-successor-history.py ;;
 butler.consolidation.native-history.v4) v3=1; history_script=memory-consolidation-v3-history.py ;;
 *) exit 2 ;;
esac
umask 077
exec 9>/srv/storage/butler/agents/native-provider-auth.lock
flock -n 9
# Re-read every fixed experimental ledger under the same auth lock. A changed
# current-day population invalidates this preseal before credentials are read.
python3 "$src/scripts/dev/$history_script" check-current "$history/history.json" "$history_hash" > "$proof/pre-auth-current-ledgers.json"
if [[ "$v2" == 1 || "$v3" == 1 ]]; then
 docker run --rm --pull never --network none --cpus 1 --memory 2g --pids-limit 128 \
  -v "$src:/src:ro" -v "$proof:/proof:ro" -v "$history:/history:ro" -w /src \
  -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker -e BUTLER_CONSOLIDATION_NATIVE_V2_CHECK=1 \
  -e BUTLER_CONSOLIDATION_NATIVE_HISTORY=/history/history.json \
  -e BUTLER_CONSOLIDATION_NATIVE_EXPECTED_POLICY=/proof/native-budget-policy.json \
  golang:1.27.2-bookworm /proof/orchestrator-app.test -test.v -test.run '^TestConsolidationV2CheckPreparedPins$' \
  > "$proof/pre-auth-successor-pins.txt" 2>&1
fi
# Reprove the frozen original history/current UTC day before reading any auth.
# This consumes no provider operation and cannot change the prepared allowance.
docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 128 \
 -v "$src:/src:ro" -v "$proof:/proof" -v "$history:/history:ro" -w /src \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_CONSOLIDATION_NATIVE_HISTORY=/history/history.json \
 -e BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256="$history_hash" \
 golang:1.27.2-bookworm /proof/orchestrator-app.test -test.v -test.run '^TestConsolidationNativeHistoryCarryIntegration$' \
 > "$proof/pre-auth-day-check.txt" 2>&1
python3 "$src/scripts/dev/memory-consolidation-auth.py" clone "$base" "$label" > "$proof/clone-safe.json"
private=$base/private/$label
handoff(){
 status=$?
 trap - EXIT
 docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof || true
 python3 "$src/scripts/dev/memory-consolidation-auth.py" return "$base" "$label" > "$proof/return-safe.json" || exit 1
 docker run --rm --pull never --network container:butler-lab-orchestrator-1 --env-file "$private/main-diagnose.env" -v "$auth_binary:/provider-auth:ro" golang:1.27.2-bookworm /provider-auth -action diagnose-cloudcode > "$private/main-diagnosis.json" || exit 1
 jq -e -f "$src/scripts/dev/memory-consolidation-main-auth.jq" "$private/main-diagnosis.json" >/dev/null || exit 1
 jq '{resolved,unsafe_refresh,expired:.after.expired,trace}' "$private/main-diagnosis.json" > "$proof/main-auth-safe.json"
 python3 "$src/scripts/dev/memory-consolidation-auth.py" snapshot "$base" "$label" > "$proof/main-snapshot-safe.json" || exit 1
 printf '%s\n' "$status" > "$proof/native-status"
 exit "$status"
}
trap handoff EXIT
docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges --env-file "$private/native.env" \
 -v "$src:/src:ro" -v "$proof:/proof" -v "$history:/history:ro" -v "$private:/private:ro" -w /src \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker \
 -e BUTLER_CONSOLIDATION_NATIVE_V1="$v1" -e BUTLER_CONSOLIDATION_NATIVE_V2="$v2" -e BUTLER_CONSOLIDATION_SCREEN_PROOF=/proof/screen \
 -e BUTLER_CONSOLIDATION_NATIVE_V3="$v3" \
 -e BUTLER_CONSOLIDATION_SCREEN_USE_SEALED_SCHEDULE=1 \
 -e BUTLER_CONSOLIDATION_NATIVE_AUTH_VALUE_FILE=/private/auth-import.value \
 -e BUTLER_CONSOLIDATION_NATIVE_HISTORY=/history/history.json -e BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256="$history_hash" \
 -e BUTLER_CONSOLIDATION_NATIVE_EXPECTED_POLICY=/proof/native-budget-policy.json \
 -e HTTP_PROXY=http://127.0.0.1:18080 -e HTTPS_PROXY=http://127.0.0.1:18080 -e ALL_PROXY=http://127.0.0.1:18080 -e NO_PROXY=localhost,127.0.0.1,::1 \
 golang:1.27.2-bookworm /proof/orchestrator-app.test -test.v -test.timeout 530m -test.run '^TestNativeConsolidationSelectedSourcePairsIntegration$' \
 > "$proof/native.txt" 2>&1

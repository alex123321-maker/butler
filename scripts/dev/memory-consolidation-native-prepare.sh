#!/usr/bin/env bash
# Offline source/binary/policy/history/schedule seal. No authentication is read.
set -euo pipefail
base=${1:?isolated issue89 root}
label=${2:?fresh prospective native label}
history=${3:?read-only complete history directory}
history_hash=${4:?reviewed exact history sha256}
[[ "$base" =~ ^/srv/storage/butler/agents/issue89-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ && "$history" == "$base/reports/"* && "$history_hash" =~ ^[a-f0-9]{64}$ ]] || exit 2
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?isolated *_test PostgreSQL required for carry proof}"
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof" "$base/.cache/go-build" "$base/.cache/go-tmp"
test "$(sha256sum "$history/history.json" | cut -d' ' -f1)" = "$history_hash"
history_format=$(jq -er .format "$history/history.json")
v2=0
v3=0
case "$history_format" in
 butler.consolidation.native-history.v2) history_script=memory-consolidation-history.py ;;
 butler.consolidation.native-history.v3)
  v2=1
  history_script=memory-consolidation-successor-history.py
  [[ "${BUTLER_CONSOLIDATION_SOURCE_REVISION:-}" =~ ^[a-f0-9]{40}$ ]] || exit 2
  ;;
 butler.consolidation.native-history.v4)
  v2=1
  v3=1
  history_script=memory-consolidation-v3-history.py
  [[ "${BUTLER_CONSOLIDATION_SOURCE_REVISION:-}" =~ ^[a-f0-9]{40}$ ]] || exit 2
  ;;
 *) exit 2 ;;
esac
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
printf '%s\n' "$src" > "$proof/source-reference"
status=0
docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -v "$history:/history:ro" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_CONSOLIDATION_SCREEN_PROOF=/proof/screen \
 -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker \
 -e BUTLER_CONSOLIDATION_NATIVE_V2_SEAL="$v2" -e BUTLER_CONSOLIDATION_SOURCE_REVISION \
 -e BUTLER_CONSOLIDATION_V3_PREPARE="$v3" -e BUTLER_CONSOLIDATION_V3_HISTORY_TEST_ROOT=/history \
 -e BUTLER_CONSOLIDATION_NATIVE_HISTORY=/history/history.json -e BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256="$history_hash" \
 -e BUTLER_CONSOLIDATION_NATIVE_POLICY_PROOF=/proof/native-budget-policy.json -e BUTLER_CONSOLIDATION_NATIVE_SEAL=1 \
 golang:1.27.2-bookworm bash -eu -c 'go build -o /proof/tool-broker ./apps/tool-broker
  go test -c -race -o /proof/orchestrator-app.test ./apps/orchestrator/internal/app
  /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationNativeSealSchedule$"
  /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationNative(FrozenHistory|HistoryCarryIntegration)$"
  /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationScreen(HistoryRejects|ProspectiveDay)"
  if [[ "$BUTLER_CONSOLIDATION_NATIVE_V2_SEAL" == 1 ]]; then
   /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationV2(LatestLeafCountsOriginalAttemptsOnce|RetainsNativeUnknownAndAdmittedWithoutEntry|RejectsChangedWitnessesOrExtraOwnerRows|CurrentDayCarryOneUseIntegration|WritePins)$"
  fi
  if [[ "$BUTLER_CONSOLIDATION_V3_PREPARE" == 1 ]]; then
   /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationV3(WorkerRejectsOwnerControlIdentity|LatestTemporalLeafCountsOnce|CurrentDayCarryOneUseIntegration|FrozenHistoryRejectsForgeries)$"
  fi' \
 > "$proof/offline-preparation.txt" 2>&1 || status=$?
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
test "$(grep -c '^--- PASS: TestConsolidationNative' "$proof/offline-preparation.txt")" = 3
! grep -Eq '^--- (FAIL|SKIP)' "$proof/offline-preparation.txt"
if [[ "$v2" == 1 ]]; then
 test "$(grep -c '^--- PASS: TestConsolidationV2' "$proof/offline-preparation.txt")" = 5
fi
if [[ "$v3" == 1 ]]; then
 test "$(grep -c '^--- PASS: TestConsolidationV3' "$proof/offline-preparation.txt")" = 4
fi
python3 "$src/scripts/dev/$history_script" check-current "$history/history.json" "$history_hash" > "$proof/current-after-build.json"
printf '%s\n' "$history" > "$proof/history-reference"
printf '%s\n' "$history_hash" > "$proof/history.sha256"
sha256sum "$proof/source.sha256" "$proof/source-reference" "$proof/tool-broker" "$proof/orchestrator-app.test" "$proof/native-budget-policy.json" "$proof/screen/initial-schedule.json" "$proof/history.sha256" "$proof/history-reference" > "$proof/prepared-pins.sha256"
if [[ "$v2" == 1 ]]; then
 sha256sum "$proof/successor-pins.json" >> "$proof/prepared-pins.sha256"
fi
# Prepare only the isolated private parent. The fresh per-window child is still
# created exclusively by the later authorized fresh-clone operation.
test ! -L "$base/private"
mkdir -p "$base/private"
chmod 700 "$base/private"
printf 'PASS\n' > "$proof/prepared"
sha256sum "$proof/prepared-pins.sha256"

#!/usr/bin/env bash
# New v6 source/binary/contract/seven-store history/full12 freeze; no auth.
set -euo pipefail
base=${1:?isolated issue88 root}
label=${2:?fresh native label with history already exported}
release=${3:?exact committed source revision}
[[ "$base" =~ ^/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+$ && "$label" =~ ^[A-Za-z0-9_-]+$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test -s "$proof/history/history.json"
test ! -e "$proof/frozen-pins.json"
test ! -e "$proof/prepared"
umask 077
mkdir -p "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
printf '%s\n' "$src" > "$proof/source-reference"
history_hash=$(sha256sum "$proof/history/history.json" | cut -d' ' -f1)
version=$(jq -er .format "$proof/history/history.json")
history_script=memory-temporal-successor-history.py
v7=0;v8=0
case "$version" in
 butler.temporal.native-history.v2) ;;
 butler.temporal.native-history.v3) history_script=memory-temporal-v7-history.py;v7=1 ;;
 butler.temporal.native-history.v4) history_script=memory-temporal-v8-history.py;v8=1 ;;
 *) exit 2 ;;
esac
python3 "$src/scripts/dev/$history_script" check-current "$proof/history/history.json" "$history_hash" > "$proof/current-before-build.json"
status=0
docker run --rm --pull never --network none --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$proof:/proof" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEMPORAL_WRITE_PINS=1 -e BUTLER_TEMPORAL_NATIVE_PROOF=/proof -e BUTLER_TEMPORAL_SOURCE_REVISION="$release" \
 -e BUTLER_TEMPORAL_V6_HISTORY_TEST_ROOT=/proof/history \
 -e BUTLER_TEMPORAL_V7_HISTORY_TEST_ROOT=/proof/history -e BUTLER_TEMPORAL_PREPARE_V7="$v7" \
 -e BUTLER_TEMPORAL_V8_HISTORY_TEST_ROOT=/proof/history -e BUTLER_TEMPORAL_PREPARE_V8="$v8" \
 -e BUTLER_TEMPORAL_CHECK_PINS=1 \
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/proof/broker.test -w /src golang:1.27.2-bookworm bash -eu -c \
 'go build -o /proof/test2json cmd/test2json
  go test -c -race -o /proof/broker.test ./apps/tool-broker/internal/broker
  go test -c -race -o /proof/collector.test ./apps/orchestrator/internal/app
  cd /src/apps/orchestrator/internal/app
  /proof/collector.test -test.v -test.run "^TestTemporalV5(CurrentDayPositiveCarryRetainsUnknownUsage|PositiveCarryRejectsOmissionRefundAndOtherPopulations)$"
  if [[ "$BUTLER_TEMPORAL_PREPARE_V8" == 1 ]];then
   /proof/collector.test -test.v -test.run "^TestClosedTemporalV7(RetainsProductNativeIndependence|RejectsConsistentInvalidSequence)$"
   /proof/collector.test -test.v -test.run "^TestTemporalV8(FactsContractBoundary|FrozenHistoryRejectsForgeries)$"
   /proof/collector.test -test.v -test.run "^TestTemporalV6WorkerRejectsOwnerControlIdentity$"
  elif [[ "$BUTLER_TEMPORAL_PREPARE_V7" == 1 ]];then
   /proof/collector.test -test.v -test.run "^TestTemporalV7(LatestLeafRetainsAdmissionWithoutNative|LatestLeafRejectsForgedWitnesses|OriginalSlotEntryCeiling|NativeCallSequence|FrozenHistoryRejectsForgeries)$"
   /proof/collector.test -test.v -test.run "^TestTemporalV6WorkerRejectsOwnerControlIdentity$"
  else
   /proof/collector.test -test.v -test.run "^TestTemporalV6(FrozenHistoryRejectsForgeries|WorkerRejectsOwnerControlIdentity)$"
  fi
  /proof/collector.test -test.v -test.run "^TestTemporalNativeEntryFilenameCapacity$"
  /proof/collector.test -test.v -test.run "^TestTemporalNativeWritePins$"
  /proof/collector.test -test.v -test.run "^TestTemporalNativeCheckPreparedPins$"' > "$proof/offline-preparation.txt" 2>&1 || status=$?
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
! grep -Eq '^--- (FAIL|SKIP)' "$proof/offline-preparation.txt"
test "$(grep -c '^--- PASS: TestTemporalV5' "$proof/offline-preparation.txt")" = 2
if [[ "$v8" == 1 ]];then
 test "$(grep -c '^--- PASS: TestClosedTemporalV7' "$proof/offline-preparation.txt")" = 2
 test "$(grep -c '^--- PASS: TestTemporalV8' "$proof/offline-preparation.txt")" = 2
 test "$(grep -c '^--- PASS: TestTemporalV6' "$proof/offline-preparation.txt")" = 1
elif [[ "$v7" == 1 ]];then
 test "$(grep -c '^--- PASS: TestTemporalV7' "$proof/offline-preparation.txt")" = 5
 test "$(grep -c '^--- PASS: TestTemporalV6' "$proof/offline-preparation.txt")" = 1
else
 test "$(grep -c '^--- PASS: TestTemporalV6' "$proof/offline-preparation.txt")" = 2
fi
grep -q '^--- PASS: TestTemporalNativeEntryFilenameCapacity' "$proof/offline-preparation.txt"
test "$(grep -c '^--- PASS: TestTemporalNativeWritePins' "$proof/offline-preparation.txt")" = 1
grep -q '^--- PASS: TestTemporalNativeCheckPreparedPins' "$proof/offline-preparation.txt"
# Exercise the exact nonroot runner before authentication. The standalone Go
# standard tool requires no module startup, writable HOME or build cache.
docker run --rm --pull never --network none --user "$(id -u):$(id -g)" --cpus 1 --memory 2g --pids-limit 128 \
 -v "$src:/src:ro" -v "$proof:/proof:ro" -w /src/apps/orchestrator/internal/app \
 golang:1.27.2-bookworm /proof/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /proof/collector.test -test.v -test.run '^TestTemporalNativeEntryFilenameCapacity$' > "$proof/nonroot-runner-preflight.jsonl" 2> "$proof/nonroot-runner-preflight.log"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestTemporalNativeEntryFilenameCapacity") and all(.[];.Action!="skip" and .Action!="fail")' "$proof/nonroot-runner-preflight.jsonl" >/dev/null
python3 "$src/scripts/dev/$history_script" check-current "$proof/history/history.json" "$history_hash" > "$proof/current-after-build.json"
sha256sum "$proof/source.sha256" "$proof/source-reference" "$proof/collector.test" "$proof/broker.test" "$proof/test2json" "$proof/nonroot-runner-preflight.jsonl" "$proof/shared-policy.json" "$proof/scheduled-before-auth.json" "$proof/frozen-pins.json" > "$proof/prepared-pins.sha256"
printf 'PASS\n' > "$proof/prepared"
sha256sum "$proof/prepared-pins.sha256"

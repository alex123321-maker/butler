#!/usr/bin/env bash
# New v5 source/binary/contract/day/history/full12 schedule freeze; no auth.
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
python3 "$src/scripts/dev/memory-temporal-history.py" check-current "$proof/history/history.json" "$history_hash" > "$proof/current-before-build.json"
status=0
docker run --rm --pull never --network none --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$proof:/proof" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEMPORAL_WRITE_PINS=1 -e BUTLER_TEMPORAL_NATIVE_PROOF=/proof -e BUTLER_TEMPORAL_SOURCE_REVISION="$release" \
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/proof/broker.test -w /src golang:1.27.2-bookworm bash -eu -c \
 'go test -c -race -o /proof/broker.test ./apps/tool-broker/internal/broker
  go test -c -race -o /proof/collector.test ./apps/orchestrator/internal/app
  cd /src/apps/orchestrator/internal/app
  /proof/collector.test -test.v -test.run "^TestTemporalV5(CurrentDayPositiveCarryRetainsUnknownUsage|PositiveCarryRejectsOmissionRefundAndOtherPopulations)$"
  /proof/collector.test -test.v -test.run "^TestTemporalNativeWritePins$"' > "$proof/offline-preparation.txt" 2>&1 || status=$?
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
! grep -Eq '^--- (FAIL|SKIP)' "$proof/offline-preparation.txt"
test "$(grep -c '^--- PASS: TestTemporalV5' "$proof/offline-preparation.txt")" = 2
test "$(grep -c '^--- PASS: TestTemporalNativeWritePins' "$proof/offline-preparation.txt")" = 1
python3 "$src/scripts/dev/memory-temporal-history.py" check-current "$proof/history/history.json" "$history_hash" > "$proof/current-after-build.json"
sha256sum "$proof/source.sha256" "$proof/source-reference" "$proof/collector.test" "$proof/broker.test" "$proof/shared-policy.json" "$proof/scheduled-before-auth.json" "$proof/frozen-pins.json" > "$proof/prepared-pins.sha256"
printf 'PASS\n' > "$proof/prepared"
sha256sum "$proof/prepared-pins.sha256"

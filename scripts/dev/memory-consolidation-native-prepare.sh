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
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
printf '%s\n' "$src" > "$proof/source-reference"
status=0
docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -v "$history:/history:ro" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_CONSOLIDATION_SCREEN_PROOF=/proof/screen \
 -e BUTLER_CONSOLIDATION_NATIVE_HISTORY=/history/history.json -e BUTLER_CONSOLIDATION_NATIVE_HISTORY_SHA256="$history_hash" \
 -e BUTLER_CONSOLIDATION_NATIVE_POLICY_PROOF=/proof/native-budget-policy.json -e BUTLER_CONSOLIDATION_NATIVE_SEAL=1 \
 golang:1.27.2-bookworm bash -c 'go build -o /proof/tool-broker ./apps/tool-broker && go test -c -race -o /proof/orchestrator-app.test ./apps/orchestrator/internal/app && /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationNativeSealSchedule$" && /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationNative(FrozenHistory|HistoryCarryIntegration)$" && /proof/orchestrator-app.test -test.v -test.run "^TestConsolidationScreen(HistoryRejects|ProspectiveDay)"' \
 > "$proof/offline-preparation.txt" 2>&1 || status=$?
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
test "$(grep -c '^--- PASS: TestConsolidationNative' "$proof/offline-preparation.txt")" = 3
! grep -Eq '^--- (FAIL|SKIP)' "$proof/offline-preparation.txt"
printf '%s\n' "$history" > "$proof/history-reference"
printf '%s\n' "$history_hash" > "$proof/history.sha256"
sha256sum "$proof/source.sha256" "$proof/source-reference" "$proof/tool-broker" "$proof/orchestrator-app.test" "$proof/native-budget-policy.json" "$proof/screen/initial-schedule.json" "$proof/history.sha256" "$proof/history-reference" > "$proof/prepared-pins.sha256"
printf 'PASS\n' > "$proof/prepared"
sha256sum "$proof/prepared-pins.sha256"

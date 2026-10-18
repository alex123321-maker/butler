#!/usr/bin/env bash
# Offline #90 source, immutable executable, CLOSED history and full57 freeze.
# This script performs read-only inventory checks and never clones credentials.
set -euo pipefail
base=${1:?isolated issue90 root}
label=${2:?fresh prepared label with history already exported}
release=${3:?exact committed source revision}
[[ "$base" =~ ^/srv/storage/butler/agents/issue90-[A-Za-z0-9_-]+$ && "$label" =~ ^[a-z0-9-]+$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test -s "$proof/history/history.json"
test ! -e "$proof/prepared"
test ! -e "$proof/plan"
test ! -e "$proof/bin"
test ! -e "$base/private/$label"
umask 077
mkdir -p "$base/private" "$base/.cache/go-build" "$base/.cache/go-tmp" "$proof/bin"
test "$(stat -c '%a' "$base/private")" = 700
test "$(stat -c '%u' "$base/private")" = "$(id -u)"
test ! -L "$base/private"
printf '%s\n' "$src" > "$proof/source-reference"
printf '%s\n' "$release" > "$proof/source-revision"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
history_hash=$(sha256sum "$proof/history/history.json" | cut -d' ' -f1)
case "$(jq -er '[.format, (.version // 0)] | @tsv' "$proof/history/history.json")" in
 $'butler.memory.closed-owner-leaf.v1\t0') history_script=memory-closed-temporal-v7-history.py ;;
 $'butler.memory.closed-owner-leaf.v2\t8') history_script=memory-closed-temporal-v8-history.py ;;
 $'butler.memory.closed-owner-leaf.v2\t9') history_script=memory-closed-temporal-v9-history.py ;;
 *) echo 'unsupported CLOSED latest-history recipe' >&2; exit 2 ;;
esac
printf '%s\n' "$history_script" > "$proof/history-checker"
python3 "$src/scripts/dev/$history_script" check-current "$proof/history/history.json" "$history_hash" > "$proof/current-before-build.json"
cache=${BUTLER_EXPERIENCE_CACHE:-$base/.cache}
mkdir -p "$cache/go-build" "$cache/go-tmp"
docker run --rm --pull never --network none --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$proof:/proof" -v "$cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 golang:1.27.2-bookworm bash -euo pipefail -c \
 'go build -o /proof/bin/test2json cmd/test2json
  go test -c -o /proof/bin/broker-experience.test ./apps/tool-broker/internal/broker
  go build -o /proof/bin/tool-broker-experience ./apps/tool-broker
  go test -c -o /proof/bin/extension-experience.test ./apps/tool-extension/internal/runtime
  go test -c -o /proof/bin/http-experience.test ./apps/tool-http/internal/runtime
  go test -race -c -o /proof/bin/app.test ./apps/orchestrator/internal/app
  chmod 0555 /proof/bin/*' > "$proof/build.log" 2>&1
# Use the exact nonroot invocation and immutable mounts before auth. Neither a
# module download nor a writable executable/cache is needed at execution time.
runner=(docker run --rm --pull never --network none --user "$(id -u):$(id -g)" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$proof:/proof" -v "$proof/bin:/artifacts:ro" -w /src/apps/orchestrator/internal/app
 -e BUTLER_BROKER_E2E_BINARY=/artifacts/tool-broker-experience -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker-experience.test
 -e BUTLER_EXTENSION_E2E_BINARY=/artifacts/extension-experience.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/artifacts/http-experience.test
 -e BUTLER_EXPERIENCE_NATIVE_HISTORY=/proof/history/history.json -e BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256="$history_hash"
 -e BUTLER_EXPERIENCE_NATIVE_PREPARED=/proof -e BUTLER_EXPERIENCE_NATIVE_PREPARE=1
 -e BUTLER_EXPERIENCE_NATIVE_EXECUTION_SLOTS="${BUTLER_EXPERIENCE_NATIVE_EXECUTION_SLOTS:-57}"
 golang:1.27.2-bookworm)
"${runner[@]}" /artifacts/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /artifacts/app.test -test.v=test2json -test.timeout=3m -test.run '^TestNativeExperiencePrepareWholeComparison$' > "$proof/offline-preparation.jsonl" 2> "$proof/offline-preparation.log"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestNativeExperiencePrepareWholeComparison") and all(.[];.Action!="fail" and .Action!="skip")' "$proof/offline-preparation.jsonl" >/dev/null
plan_hash=$(sha256sum "$proof/plan/preseal.json" | cut -d' ' -f1)
docker run --rm --pull never --network none --user "$(id -u):$(id -g)" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$proof:/proof:ro" -v "$proof/bin:/artifacts:ro" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_BROKER_E2E_BINARY=/artifacts/tool-broker-experience -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker-experience.test \
 -e BUTLER_EXTENSION_E2E_BINARY=/artifacts/extension-experience.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/artifacts/http-experience.test \
 -e BUTLER_EXPERIENCE_NATIVE_HISTORY=/proof/history/history.json -e BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256="$history_hash" \
 -e BUTLER_EXPERIENCE_NATIVE_CHECK=1 -e BUTLER_EXPERIENCE_NATIVE_PLAN=/proof/plan/preseal.json -e BUTLER_EXPERIENCE_NATIVE_PLAN_SHA256="$plan_hash" \
 golang:1.27.2-bookworm /artifacts/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /artifacts/app.test -test.v=test2json -test.timeout=3m -test.run '^TestNativeExperienceCheckPreparedWholeComparison$' > "$proof/offline-check.jsonl" 2> "$proof/offline-check.log"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestNativeExperienceCheckPreparedWholeComparison") and all(.[];.Action!="fail" and .Action!="skip")' "$proof/offline-check.jsonl" >/dev/null
python3 "$src/scripts/dev/$history_script" check-current "$proof/history/history.json" "$history_hash" > "$proof/current-after-build.json"
find "$proof/history" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/history.sha256"
sha256sum "$proof/bin/"* > "$proof/binaries.sha256"
sha256sum "$proof/source-reference" "$proof/source-revision" "$proof/source.sha256" "$proof/history-checker" "$proof/history.sha256" "$proof/binaries.sha256" "$proof/plan/preseal.json" "$proof/plan/schedule.json" "$proof/offline-preparation.jsonl" "$proof/offline-check.jsonl" > "$proof/prepared-pins.sha256"
printf 'PASS\n' > "$proof/prepared"
sha256sum "$proof/plan/preseal.json" "$proof/prepared-pins.sha256"

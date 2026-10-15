#!/usr/bin/env bash
# Actual local query during owned index slice. No cloud credentials or downloads.
set -euo pipefail
base=${1:?isolated issue92 root}
label=${2:?fresh report label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label; test ! -e "$proof"; umask 077
# Only the isolated synthetic PostgreSQL fixture, never main. The Go test
# creates and removes its own cryptographically random schema.
pg=${BUTLER_CAPACITY_LOCAL_PG_IP:?private isolated synthetic PostgreSQL IPv4}
model=${BUTLER_CAPACITY_LOCAL_MODEL_IP:?private operator-pinned local-models IPv4}
private_ip() { [[ "$1" =~ ^10\.[0-9]+\.[0-9]+\.[0-9]+$ || "$1" =~ ^172\.(1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+$ || "$1" =~ ^192\.168\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; }
private_ip "$pg" && private_ip "$model" || exit 2
token=${BUTLER_CAPACITY_LOCAL_TOKEN_FILE:-/srv/storage/butler/agents/local-models/private/token}
[[ "$token" == /srv/storage/butler/agents/local-models/private/* && -f "$token" ]] || exit 2
mkdir -p "$proof" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
uname -a > "$proof/hardware.txt"; lscpu >> "$proof/hardware.txt"
runner=butler-issue92-$label-local
stats_pid=''
cleanup(){ [[ -z "$stats_pid" ]] || kill "$stats_pid" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
(while true; do date -u +%Y-%m-%dT%H:%M:%SZ; docker stats --no-stream --format '{{json .}}' "$runner" 2>/dev/null || true; sleep 5; done) > "$proof/resources.log" & stats_pid=$!
docker run --rm --pull never --name "$runner" --network host --add-host "local-models:$model" --cpus 1 --memory 2g --pids-limit 128 \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -v "$token:/local-token:ro" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_MEMORY_CAPACITY_INTERACTIVE=1 -e BUTLER_MEMORY_CAPACITY_REPORT=/proof \
 -e BUTLER_MEMORY_SEMANTIC_ENABLED=true -e BUTLER_MEMORY_LOCAL_SERVICE_ID=operator-pinned-service -e BUTLER_MEMORY_LOCAL_TOKEN_FILE=/local-token \
 -e "BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler:synthetic-test@$pg:5432/memory_read_test?sslmode=disable" \
 golang:1.27.2-bookworm go test -json -race -p 1 -count=1 -timeout 15m -run '^TestMemoryCapacityActualForegroundDuringOwnedIndex$' ./apps/orchestrator/internal/app > "$proof/tests.jsonl" 2> "$proof/build.log"
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/tests.jsonl"; then exit 1; fi
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestMemoryCapacityActualForegroundDuringOwnedIndex")' "$proof/tests.jsonl" >/dev/null
sha256sum "$proof/source.sha256" "$proof/tests.jsonl" "$proof/actual-interactive-index.json" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"

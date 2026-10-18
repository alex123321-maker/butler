#!/usr/bin/env bash
# Offline history-correspondence and actual PG concurrent carry controls only.
set -euo pipefail
base=${1:?isolated issue89 root}
label=${2:?fresh controlled-history label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+$ && "$label" =~ ^[A-Za-z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
network=butler-issue89-$label-history-net
pg=butler-issue89-$label-history-pg
created_network=false
created_pg=false
cleanup(){
 if $created_pg; then
  docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
  docker rm -f "$pg" >/dev/null 2>&1 || true
  created_pg=false
 fi
 if $created_network; then docker network rm "$network" >/dev/null 2>&1 || true; created_network=false; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
docker network create --internal --subnet 10.237.201.0/28 "$network" > "$proof/network-id"
created_network=true
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 1g --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=consolidation_history_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=64MB -c max_connections=24 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60);do
 docker exec "$pg" pg_isready -h 127.0.0.1 -U butler -d consolidation_history_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
status=0
docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/consolidation_history_test?sslmode=disable' \
 golang:1.27.2-bookworm bash -eu -c 'go test -json -race -p 1 -count=1 -timeout 3m -run "^TestConsolidationV2(LatestLeafCountsOriginalAttemptsOnce|RetainsNativeUnknownAndAdmittedWithoutEntry|RejectsChangedWitnessesOrExtraOwnerRows|CurrentDayCarryOneUseIntegration)$" ./apps/orchestrator/internal/app > /proof/events.jsonl
  go vet ./apps/orchestrator/internal/app > /proof/vet.log' > "$proof/runner.log" 2>&1 || status=$?
cleanup
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
for suffix in LatestLeafCountsOriginalAttemptsOnce RetainsNativeUnknownAndAdmittedWithoutEntry RejectsChangedWitnessesOrExtraOwnerRows CurrentDayCarryOneUseIntegration;do
 jq -s -e --arg name "TestConsolidationV2$suffix" 'any(.[];.Action=="pass" and .Test==$name) and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/events.jsonl" >/dev/null
done
printf '{"controlled_only":true,"native_calls":0,"auth_imported":false,"client_cpu":1,"client_memory_bytes":2147483648,"postgres_cpu":1,"postgres_memory_bytes":1073741824,"separate_pg":true,"six_native_stores_modified":false,"compiler_and_modules":"cached_offline"}\n' > "$proof/environment.json"
sha256sum "$proof/source.sha256" "$proof/events.jsonl" "$proof/vet.log" "$proof/environment.json" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf 'History witnesses and concurrent actual-PG one-use import passed; native calls 0, auth 0.\n'

#!/usr/bin/env bash
# Real ordinary foreground versus real memory consumers; controlled provider.
set -euo pipefail
base=${1:?isolated issue92 root}
label=${2:?fresh report label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label; test ! -e "$proof"; umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
network=butler-issue92-$label-priority-net; pg=butler-issue92-$label-priority-pg
created_pg=false; created_net=false
cleanup(){
 $created_pg && docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet 10.237.196.0/24 "$network" > "$proof/network-id"; created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 1g --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_background_test \
 pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id"; created_pg=true
for i in $(seq 1 60); do docker exec "$pg" pg_isready -U butler -d memory_background_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
runner=(docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_background_test?sslmode=disable'
 -e BUTLER_MEMORY_READ_APP_BINARY=/cache/tool-broker-background
 -e BUTLER_MEMORY_CAPACITY_BACKGROUND=1 -e BUTLER_MEMORY_CAPACITY_BACKGROUND_REPORT=/proof
 -e BUTLER_MEMORY_COHERENT_PROOF=/proof/coherent
 golang:1.27.2-bookworm)
"${runner[@]}" bash -eu -c 'go build -o /cache/tool-broker-background ./apps/tool-broker; go test -json -race -p 1 -count=1 -timeout 8m -run "^TestMemoryCapacityForegroundDuring(Compaction|Consolidation)Integration$" ./apps/orchestrator/internal/app > /proof/tests.jsonl; go vet ./apps/orchestrator/internal/app > /proof/vet.log' > "$proof/runner.log" 2>&1
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/tests.jsonl"; then exit 1; fi
for name in Compaction Consolidation; do
 jq -s -e --arg name "TestMemoryCapacityForegroundDuring${name}Integration" 'any(.[];.Action=="pass" and .Test==$name)' "$proof/tests.jsonl" >/dev/null
done
for name in compaction consolidation; do
 jq -e '.controlled_provider and .native_calls==0 and .foreground_millis<=5000 and .foreground_answer=="4" and .quote==.charged and .retained_attempt.usage==null and .foreground_input.status=="completed"' "$proof/$name.json" >/dev/null
done
sha256sum "$proof/source.sha256" "$proof/tests.jsonl" "$proof/compaction.json" "$proof/consolidation.json" "$proof/coherent/timeline.json" "$proof/vet.log" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf 'Two controlled ordinary foreground/consumer contention slices passed. Native calls0.\n'

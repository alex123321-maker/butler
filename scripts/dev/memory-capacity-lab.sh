#!/usr/bin/env bash
# Synthetic guarded Store capacity screen; no credentials/inference/internet.
set -euo pipefail
base=${1:?isolated issue91 root}
label=${2:?fresh report label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue91-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current");[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label;test ! -e "$proof";umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
uname -a > "$proof/hardware.txt";lscpu >> "$proof/hardware.txt";free -b >> "$proof/hardware.txt";df -B1 /srv/storage >> "$proof/hardware.txt"
network=butler-issue91-$label-net;pg=butler-issue91-$label-pg
created_pg=false;created_net=false;stats_pid=''
cleanup(){
 [[ -z "$stats_pid" ]] || kill "$stats_pid" 2>/dev/null || true
 $created_pg && docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet 10.237.91.0/24 "$network" > "$proof/network-id";created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 1g --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_capacity_test \
 pgvector/pgvector:pg17 -c shared_buffers=128MB -c max_connections=20 -c max_wal_size=128MB > "$proof/postgres-id";created_pg=true
for i in $(seq 1 60);do docker exec "$pg" pg_isready -U butler -d memory_capacity_test >/dev/null 2>&1 && break;[[ "$i" != 60 ]] || exit 1;sleep 1;done
docker exec "$pg" psql -U butler -d memory_capacity_test -Atc "SELECT version();CREATE EXTENSION vector;SELECT extversion FROM pg_extension WHERE extname='vector';" > "$proof/versions.txt"
runner=butler-issue91-$label-client
(while true;do date -u +%Y-%m-%dT%H:%M:%SZ;docker stats --no-stream --format '{{json .}}' "$pg" "$runner" 2>/dev/null || true;sleep 5;done) > "$proof/resources.jsonl" & stats_pid=$!
docker run --rm --pull never --name "$runner" --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_MEMORY_CAPACITY_SCREEN=1 -e BUTLER_MEMORY_CAPACITY_REPORT=/proof \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_capacity_test?sslmode=disable' \
 golang:1.27.2-bookworm go test -json -p 1 -count=1 -timeout 80m -run '^TestMemoryCapacityScreen$' ./apps/orchestrator/internal/app > "$proof/tests.jsonl" 2> "$proof/build.log"
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/tests.jsonl";then exit 1;fi
sha256sum "$proof/source.sha256" "$proof/tests.jsonl" "$proof/"*/results.json > "$proof/evidence.sha256"
printf 'COLLECTION_COMPLETE\n' > "$proof/complete"

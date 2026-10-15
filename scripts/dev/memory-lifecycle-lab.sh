#!/usr/bin/env bash
# Issue83: actual PG/privacy and old custom-backup restore with newest anchor.
# No inference, external network, real memory or credential files are needed.
set -euo pipefail
base=${1:?isolated issue83 root required}
label=${2:?fresh evidence label required}
[[ "$base" =~ ^/srv/storage/butler/agents/issue83-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
tag=butler-issue83-$label
network=$tag-net
pg=$tag-pg
redis=$tag-redis
created_pg=false
created_redis=false
created_network=false
cleanup() {
 $created_pg && docker logs --tail 40 "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_redis && docker rm -f "$redis" >/dev/null 2>&1 || true
 $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet "${BUTLER_LIFECYCLE_SUBNET:-10.253.183.0/28}" "$network" > "$proof/network-id"
created_network=true
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 512m --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_lifecycle_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60); do
 docker exec "$pg" pg_isready -U butler -d memory_lifecycle_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
docker run -d --pull never --name "$redis" --network "$network" --network-alias redis \
 --cpus 0.5 --memory 128m --pids-limit 64 --security-opt no-new-privileges redis:7.4-alpine > "$proof/redis-id"
created_redis=true
for db in memory_legacy_test butler_memory_backup_test butler_memory_restore_test;do
 docker exec "$pg" createdb -U butler "$db"
done
run_go() {
 docker run --rm --pull never --network "$network" --cpus 2 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_lifecycle_test?sslmode=disable' \
 -e BUTLER_TEST_SEMANTIC_POSTGRES_URL='postgres://butler@postgres:5432/memory_lifecycle_test?sslmode=disable' \
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://butler@postgres:5432/memory_lifecycle_test?sslmode=disable' \
 -e BUTLER_TEST_POSTGRES_URL='postgres://butler@postgres:5432/memory_legacy_test?sslmode=disable' \
 -e BUTLER_TEST_REDIS_URL=redis://redis:6379/14 \
 -e BUTLER_TEST_MEMORY_BACKUP_MANIFEST=/proof/deletion-anchor \
 golang:1.27.2-bookworm bash -eu -c "$1"
}
run_go '
go test -json -race -p 1 -count=1 -timeout 10m -skip "^TestActualPostgresDumpRestoreDeletionIntegration$" ./internal/memory/catalog ./internal/memory/jobs ./internal/memory/semantic ./internal/memory/summary ./internal/memory/service ./internal/security/content > /proof/lifecycle.jsonl
go test -json -race -p 1 -count=1 -timeout 3m -run "^Test(MemoryLifecycle|DirectBrokerCalls|MemoryCandidateExecutor)" ./apps/tool-broker/internal/broker > /proof/broker.jsonl
go test -json -race -p 1 -count=1 -timeout 3m -run "^TestLifecycleRedisCacheWithdrawalIntegration$" ./apps/orchestrator/internal/app > /proof/cache.jsonl
go vet ./internal/memory/catalog ./internal/memory/jobs ./internal/memory/semantic ./apps/tool-broker/internal/broker
' > "$proof/runner.log" 2>&1
run_go "BUTLER_TEST_MEMORY_BACKUP_PHASE=seed BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL='postgres://butler@postgres:5432/butler_memory_backup_test?sslmode=disable' go test -json -race -count=1 -run '^TestActualPostgresDumpRestoreDeletionIntegration$' ./internal/memory/catalog > /proof/backup-seed.jsonl" >> "$proof/runner.log" 2>&1
docker exec "$pg" pg_dump -U butler -Fc butler_memory_backup_test > "$proof/before-deletion.dump"
test -s "$proof/before-deletion.dump"
run_go "BUTLER_TEST_MEMORY_BACKUP_PHASE=delete BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL='postgres://butler@postgres:5432/butler_memory_backup_test?sslmode=disable' go test -json -race -count=1 -run '^TestActualPostgresDumpRestoreDeletionIntegration$' ./internal/memory/catalog > /proof/backup-delete.jsonl" >> "$proof/runner.log" 2>&1
docker exec -i "$pg" pg_restore -U butler --exit-on-error -d butler_memory_restore_test < "$proof/before-deletion.dump"
run_go "BUTLER_TEST_MEMORY_BACKUP_PHASE=restore BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL='postgres://butler@postgres:5432/butler_memory_restore_test?sslmode=disable' go test -json -race -count=1 -run '^TestActualPostgresDumpRestoreDeletionIntegration$' ./internal/memory/catalog > /proof/backup-restore.jsonl" >> "$proof/runner.log" 2>&1
# Mandatory kernel scenarios cannot disappear, skip or be replaced by package PASS.
while IFS= read -r name;do
 [[ -z "$name" || "$name" == \#* ]] && continue
 jq -s -e --arg name "$name" 'any(.[]; .Action=="pass" and .Test==$name) and (all(.[]; .Test!=$name or (.Action!="skip" and .Action!="fail")))' "$proof/"*.jsonl >/dev/null
done < "$src/scripts/dev/memory-lifecycle-required-tests.txt"
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip")|{package:.Package,test:.Test,reason:"see raw Output events"})}' "$proof/"*.jsonl > "$proof/summary.json"
jq -e '.failures==0' "$proof/summary.json" >/dev/null
sha256sum "$proof/source.sha256" "$src/internal/memory/memorytest/testdata/lifecycle-v1.json" "$proof/"*.jsonl "$proof/before-deletion.dump" > "$proof/evidence.sha256"
# The manifest is deliberately mode600 and owned by the test process. Hash it
# as that same container user; never relax its private host-file permissions.
docker run --rm --pull never --network none -v "$proof:/proof:ro" golang:1.27.2-bookworm sha256sum /proof/deletion-anchor >> "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

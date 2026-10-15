#!/usr/bin/env bash
# Actual custom dump, deletion, independent restore with newest external anchor.
set -euo pipefail
base=${1:?isolated issue92 root}; label=${2:?fresh proof label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label; test ! -e "$proof"; umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
network=butler-issue92-$label-net; pg=butler-issue92-$label-pg
created_pg=false; created_net=false
cleanup(){
 $created_pg && docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet 10.237.193.0/24 "$network" > "$proof/network-id"; created_net=true
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 512m --pids-limit 128 \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=butler_memory_backup_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=16 -c max_wal_size=64MB > "$proof/postgres-id"; created_pg=true
for i in $(seq 1 60); do docker exec "$pg" pg_isready -U butler -d butler_memory_backup_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
docker exec "$pg" createdb -U butler butler_memory_restore_test
phase(){
 docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
  -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
  -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
  -e "BUTLER_TEST_MEMORY_BACKUP_PHASE=$1" -e "BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL=postgres://butler@postgres:5432/$2?sslmode=disable" -e BUTLER_TEST_MEMORY_BACKUP_MANIFEST=/proof/deletion-anchor \
  golang:1.27.2-bookworm go test -json -race -count=1 -timeout 3m -run '^TestActualPostgresDumpRestoreDeletionIntegration$' ./internal/memory/catalog > "$proof/$1.jsonl" 2> "$proof/$1.log"
}
phase seed butler_memory_backup_test
docker exec "$pg" pg_dump -U butler -Fc butler_memory_backup_test > "$proof/before-deletion.dump"; test -s "$proof/before-deletion.dump"
phase delete butler_memory_backup_test
docker exec -i "$pg" pg_restore -U butler --exit-on-error -d butler_memory_restore_test < "$proof/before-deletion.dump" > "$proof/restore.log" 2>&1
phase restore butler_memory_restore_test
for part in seed delete restore; do
 ! grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/$part.jsonl"
 jq -s -e 'any(.[];.Action=="pass" and .Test=="TestActualPostgresDumpRestoreDeletionIntegration")' "$proof/$part.jsonl" >/dev/null
done
sha256sum "$proof/source.sha256" "$proof/before-deletion.dump" "$proof/deletion-anchor" "$proof/seed.jsonl" "$proof/delete.jsonl" "$proof/restore.jsonl" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf '3 phases passed; no native calls\n'

#!/usr/bin/env bash
# Real accepted-lesson snapshot/episode backup restore, controlled inference only.
set -euo pipefail
base=${1:?isolated issue90 root required}
label=${2:?fresh evidence label required}
[[ "$base" =~ ^/srv/storage/butler/agents/issue90-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
cache=${BUTLER_EXPERIENCE_CACHE:-$base/.cache}
mkdir -p "$proof/postgres" "$cache/go-build" "$cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
tag=butler-experience-backup-$label
network=$tag-net
pg=$tag-pg
created_pg=false
created_network=false
cleanup() {
 $created_pg && docker logs --tail 40 "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet "${BUTLER_EXPERIENCE_BACKUP_SUBNET:-10.237.191.0/28}" "$network" > "$proof/network-id"
created_network=true
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 512m --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=experience_backup_bootstrap_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60); do
 docker exec "$pg" pg_isready -U butler -d experience_backup_bootstrap_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
for db in butler_experience_backup_test butler_experience_restore_test;do docker exec "$pg" createdb -U butler "$db";done
run_phase() {
 local phase=$1 database=$2
 docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
  --security-opt no-new-privileges \
  -v "$src:/src:ro" -v "$cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
  -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
  -e BUTLER_TEST_EXPERIENCE_BACKUP_PHASE="$phase" \
  -e BUTLER_TEST_EXPERIENCE_BACKUP_POSTGRES_URL="postgres://butler@postgres:5432/$database?sslmode=disable" \
  -e BUTLER_TEST_EXPERIENCE_BACKUP_MANIFEST=/proof/deletion-anchor \
  -e BUTLER_TEST_EXPERIENCE_BACKUP_RECEIPT=/proof/receipt.json \
  golang:1.27.2-bookworm go test -json -race -p 1 -count=1 -timeout 90s \
  -run '^TestActualPostgresExperienceDumpRestoreDeletionIntegration$' ./internal/memory/experience \
  > "$proof/$phase.jsonl" 2>&1
}
run_phase seed butler_experience_backup_test
docker exec "$pg" pg_dump -U butler -Fc butler_experience_backup_test > "$proof/before-deletion.dump"
test -s "$proof/before-deletion.dump"
run_phase delete butler_experience_backup_test
docker exec -i "$pg" pg_restore -U butler --exit-on-error -d butler_experience_restore_test < "$proof/before-deletion.dump"
run_phase restore butler_experience_restore_test
for phase in seed delete restore;do
 jq -s -e 'any(.[];.Action=="pass" and .Test=="TestActualPostgresExperienceDumpRestoreDeletionIntegration") and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/$phase.jsonl" >/dev/null
done
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/"*.jsonl > "$proof/summary.json"
jq -e '.passed==3 and .failures==0 and .skipped==0' "$proof/summary.json" >/dev/null
sha256sum "$proof/source.sha256" "$proof/"*.jsonl "$proof/before-deletion.dump" > "$proof/evidence.sha256"
docker run --rm --pull never --network none -v "$proof:/proof:ro" golang:1.27.2-bookworm sha256sum /proof/receipt.json /proof/deletion-anchor >> "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

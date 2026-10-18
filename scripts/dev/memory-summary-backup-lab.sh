#!/usr/bin/env bash
# Real consumer-specific summary backup/restore; controlled provider, no network.
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
tag=butler-summary-backup-${base##*/}-$label
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
docker network create --internal --subnet "${BUTLER_SUMMARY_BACKUP_SUBNET:-10.253.184.0/28}" "$network" > "$proof/network-id"
created_network=true
# Explicit storage bind: never let the image create an anonymous PG volume.
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 512m --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=summary_backup_bootstrap_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60); do
 docker exec "$pg" pg_isready -U butler -d summary_backup_bootstrap_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
run_phase() {
 local target=$1 phase=$2 database=$3
 docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
  --security-opt no-new-privileges \
  -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
  -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
  -e BUTLER_TEST_SUMMARY_BACKUP_PHASE="$phase" -e BUTLER_TEST_SUMMARY_BACKUP_TARGET="$target" \
  -e BUTLER_TEST_SUMMARY_BACKUP_POSTGRES_URL="postgres://butler@postgres:5432/$database?sslmode=disable" \
  -e BUTLER_SUMMARY_NATIVE_MANIFEST="/proof/$target/deletion-anchor" \
  -e BUTLER_TEST_SUMMARY_BACKUP_RECEIPT="/proof/$target/receipt.json" \
  golang:1.27.2-bookworm go test -json -race -p 1 -count=1 -timeout 90s \
  -run '^TestActualPostgresSummarySnapshotDumpRestoreDeletionIntegration$' ./internal/memory/compaction \
  > "$proof/$target/$phase.jsonl" 2>&1
}
for target in message ancestor; do
 mkdir "$proof/$target"
 backup=butler_summary_${target}_backup_test
 restored=butler_summary_${target}_restore_test
 docker exec "$pg" createdb -U butler "$backup"
 docker exec "$pg" createdb -U butler "$restored"
 run_phase "$target" seed "$backup"
 docker exec "$pg" pg_dump -U butler -Fc "$backup" > "$proof/$target/before-deletion.dump"
 test -s "$proof/$target/before-deletion.dump"
 run_phase "$target" delete "$backup"
 # The newest private anchor is outside the backup and is never replaced.
 docker exec -i "$pg" pg_restore -U butler --exit-on-error -d "$restored" < "$proof/$target/before-deletion.dump"
 run_phase "$target" restore "$restored"
 for phase in seed delete restore; do
  jq -s -e 'any(.[];.Action=="pass" and .Test=="TestActualPostgresSummarySnapshotDumpRestoreDeletionIntegration") and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/$target/$phase.jsonl" >/dev/null
 done
done
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/"{message,ancestor}/*.jsonl > "$proof/summary.json"
jq -e '.passed==6 and .failures==0 and .skipped==0' "$proof/summary.json" >/dev/null
sha256sum "$proof/source.sha256" "$proof/"{message,ancestor}/*.jsonl "$proof/"{message,ancestor}/before-deletion.dump > "$proof/evidence.sha256"
# Private receipts/anchors stay0600; hash under the same container identity.
docker run --rm --pull never --network none -v "$proof:/proof:ro" golang:1.27.2-bookworm \
 sha256sum /proof/message/receipt.json /proof/message/deletion-anchor /proof/ancestor/receipt.json /proof/ancestor/deletion-anchor >> "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

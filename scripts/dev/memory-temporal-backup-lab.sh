#!/usr/bin/env bash
# Explicit one-use old dump/fresh restore procedure; no provider or Internet.
set -euo pipefail
base=${1:?isolated issue88 root required}
label=${2:?fresh evidence label required}
snapshot=${3:?frozen snapshot label required}
[[ "$base" =~ ^/srv/storage/butler/agents/issue88-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ && "$snapshot" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/snapshots/$snapshot")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof"
pg=butler-issue85-postgres
# createdb refuses existing identities; never reset historical/native budgets.
for database in butler_temporal_backup_test butler_temporal_restore_test; do
 docker exec "$pg" createdb -h 127.0.0.1 -p 18585 -U butler "$database"
done
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
run_phase() {
 local phase=$1 database=$2
 docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 256 \
  -v "$src:/src:ro" -v /srv/storage/butler/agents/issue85-20261016/.cache:/cache \
  -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
  -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
  -e BUTLER_TEST_TEMPORAL_BACKUP_PHASE="$phase" \
  -e BUTLER_TEST_TEMPORAL_BACKUP_POSTGRES_URL="postgres://butler@127.0.0.1:18585/$database?sslmode=disable" \
  -e BUTLER_TEST_TEMPORAL_BACKUP_MANIFEST=/proof/deletion-anchor \
  -e BUTLER_TEST_TEMPORAL_BACKUP_RECEIPT=/proof/receipt.json \
  golang:1.27.2-bookworm go test -json -race -p 1 -count=1 -timeout 90s \
  -run '^TestActualPostgresTemporalDumpRestoreDeletionIntegration$' ./internal/memory/catalog > "$proof/$phase.jsonl" 2>&1
}
run_phase seed butler_temporal_backup_test
docker exec "$pg" pg_dump -h 127.0.0.1 -p 18585 -U butler -Fc butler_temporal_backup_test > "$proof/before-deletion.dump"
test -s "$proof/before-deletion.dump"
run_phase delete butler_temporal_backup_test
docker exec -i "$pg" pg_restore -h 127.0.0.1 -p 18585 -U butler --exit-on-error -d butler_temporal_restore_test < "$proof/before-deletion.dump"
run_phase restore butler_temporal_restore_test
for phase in seed delete restore; do
 jq -s -e 'any(.[];.Action=="pass" and .Test=="TestActualPostgresTemporalDumpRestoreDeletionIntegration") and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/$phase.jsonl" >/dev/null
done
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/"*.jsonl > "$proof/summary.json"
sha256sum "$proof/source.sha256" "$proof/"*.jsonl "$proof/before-deletion.dump" > "$proof/evidence.sha256"
docker run --rm --pull never --network none -v "$proof:/proof:ro" golang:1.27.2-bookworm \
 sha256sum /proof/receipt.json /proof/deletion-anchor >> "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

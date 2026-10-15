#!/usr/bin/env bash
# Actual accepted episodic projection restore, controlled transport only.
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

tag=butler-candidate-backup-${base##*/}-$label
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

docker network create --internal --subnet "${BUTLER_CANDIDATE_BACKUP_SUBNET:-10.253.185.0/28}" "$network" > "$proof/network-id"
created_network=true
# Explicit storage-backed PGDATA; no anonymous volumes or lab-main changes.
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 512m --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=candidate_backup_bootstrap_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60); do
 docker exec "$pg" pg_isready -U butler -d candidate_backup_bootstrap_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
for db in butler_candidate_episode_backup_test butler_candidate_episode_restore_test candidate_regression_test; do
 docker exec "$pg" createdb -U butler "$db"
done
run_go() {
 docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
  --security-opt no-new-privileges \
  -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
  -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
  -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/candidate_regression_test?sslmode=disable' \
  -e BUTLER_TEST_CANDIDATE_BACKUP_MANIFEST=/proof/deletion-anchor \
  -e BUTLER_TEST_CANDIDATE_BACKUP_RECEIPT=/proof/receipt.json \
  golang:1.27.2-bookworm bash -eu -c "$1"
}
run_phase() {
 local phase=$1 database=$2
 run_go "BUTLER_TEST_CANDIDATE_BACKUP_PHASE=$phase BUTLER_TEST_CANDIDATE_BACKUP_POSTGRES_URL='postgres://butler@postgres:5432/$database?sslmode=disable' go test -json -race -p 1 -count=1 -timeout 90s -run '^TestActualPostgresCandidateEpisodeDumpRestoreDeletionIntegration$' ./apps/orchestrator/internal/api" > "$proof/$phase.jsonl" 2>&1
}
run_phase seed butler_candidate_episode_backup_test
docker exec "$pg" pg_dump -U butler -Fc butler_candidate_episode_backup_test > "$proof/before-deletion.dump"
test -s "$proof/before-deletion.dump"
run_phase delete butler_candidate_episode_backup_test
# Use the newest external anchor unchanged, never the copy from the old DB.
docker exec -i "$pg" pg_restore -U butler --exit-on-error -d butler_candidate_episode_restore_test < "$proof/before-deletion.dump"
run_phase restore butler_candidate_episode_restore_test
for phase in seed delete restore; do
 jq -s -e 'any(.[];.Action=="pass" and .Test=="TestActualPostgresCandidateEpisodeDumpRestoreDeletionIntegration") and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/$phase.jsonl" >/dev/null
done
# Exercise the unchanged ordinary fixture through the same refactored helper.
run_go 'go test -json -race -p 1 -count=1 -timeout 90s -run "^TestCandidate(HTTPWorkerOwnerDecisionCorrectionIntegration|EpisodeOwnerProjectionAndRevokeIntegration|LateSourceChangeAndLocalOnlyIntegration)$" ./apps/orchestrator/internal/api' > "$proof/regression.jsonl" 2>&1
for name in TestCandidateHTTPWorkerOwnerDecisionCorrectionIntegration TestCandidateEpisodeOwnerProjectionAndRevokeIntegration TestCandidateLateSourceChangeAndLocalOnlyIntegration; do
 jq -s -e --arg name "$name" 'any(.[];.Action=="pass" and .Test==$name) and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/regression.jsonl" >/dev/null
done
run_go 'go vet ./apps/orchestrator/internal/api' > "$proof/vet.log" 2>&1
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/"*.jsonl > "$proof/summary.json"
jq -e '.passed>=6 and .failures==0 and .skipped==0' "$proof/summary.json" >/dev/null
sha256sum "$proof/source.sha256" "$proof/"*.jsonl "$proof/before-deletion.dump" > "$proof/evidence.sha256"
# Keep the independent anchor and content-free receipt private (0600).
docker run --rm --pull never --network none -v "$proof:/proof:ro" golang:1.27.2-bookworm \
 sha256sum /proof/receipt.json /proof/deletion-anchor >> "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

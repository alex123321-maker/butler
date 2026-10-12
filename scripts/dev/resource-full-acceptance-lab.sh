#!/usr/bin/env bash
# Eight-criterion resource evidence: real PG, owning Broker and actual Chrome.
set -euo pipefail
root=${1:?isolated root required}
[[ "$root" == /srv/storage/butler/agents/resource-full-acceptance-20261012 ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
stamp="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/resource-matrix-$stamp"
name="butler-resource-matrix-$stamp"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$proof/binaries" "$root/.cache/private-tmp"
export DOCKER_CONFIG="$root/.cache/docker"
created_pg=false; created_net=false
cleanup() {
  $created_pg && docker logs --tail 30 "$name" > "$proof/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal "$network" >/dev/null; created_net=true
docker run -d --name "$name" --runtime runc --network "$network" --network-alias resource-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=resource-fixture --env POSTGRES_DB=butler_resource_matrix_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=32 >/dev/null; created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$name" pg_isready -U fixture -d butler_resource_matrix_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1; sleep 1
done
printf 'Resource matrix evidence: %s\n' "$proof"
(cd "$source_dir"; find . -type f -print0 | sort -z | xargs -0 sha256sum) > "$proof/source.sha256"
dsn='postgres://fixture:resource-fixture@resource-postgres:5432/butler_resource_matrix_test?sslmode=disable'
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
  --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/private-tmp:/private-tmp" --volume "$proof/binaries:/binaries" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env "BUTLER_TEST_POSTGRES_URL=$dsn" --env "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=$dsn" --env "BUTLER_TEST_MEMORY_POSTGRES_URL=$dsn" \
  golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 -timeout=3m ./internal/resources ./internal/resourceexec ./internal/budget
go test -json -race -p 1 -count=1 -timeout=90s -run "^Test(SourceDuplicateAndConflictingPreferencesIntegration|ConcurrentExplicitSupersessionIntegration|ExtractionConcurrentWorkerCandidatesNeedOwnerHeadCASIntegration)$" ./internal/memory/catalog
go test -json -race -p 1 -count=1 -timeout=90s -run "^TestChildAdmissionCannotResetRootBudgetIntegration$" ./apps/orchestrator/internal/task
go test -c -race -p 1 -o /binaries/broker.test ./apps/tool-broker/internal/broker
go test -c -race -p 1 -o /binaries/orchestrator.test ./apps/orchestrator/internal/app
export BUTLER_GUARDED_REQUEST_BROKER_BINARY=/binaries/broker.test
go tool test2json -t -p github.com/butler/butler/apps/orchestrator/internal/app /binaries/orchestrator.test -test.v -test.timeout=3m -test.run "^TestMemoryCandidate(DispatchedIntegration|InputExpiresDuringDedupWaitIntegration|RejectsSensitiveData)$"
go vet ./internal/resources ./internal/resourceexec ./internal/budget ./internal/memory/catalog ./apps/orchestrator/internal/task ./apps/orchestrator/internal/app
' > "$proof/tests.jsonl"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl" || exit 1
sha256sum "$proof/tests.jsonl" "$proof/source.sha256" > "$proof/events.sha256"
printf 'Resource PG/Broker/budget/candidate matrix passed; native_calls0.\n'
# Separate synthetic browser profile: production extension, real Linux native
# host/owner selection/private Broker. No owner account or real browser profile.
bash "$source_dir/scripts/dev/single-tab-resource-lab.sh" "$root"

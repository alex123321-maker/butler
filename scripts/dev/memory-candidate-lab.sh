#!/usr/bin/env bash
# Disposable PostgreSQL proof of memory candidate publication; zero inference.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
check_mode=${2:-narrow}
[[ "$check_mode" == narrow || "$check_mode" == full ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/memory-candidate-$run_id"
name="butler-memory-candidate-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" /srv/storage/butler/.cache/go-tmp
created_network=false
created_pg=false
cleanup() {
 $created_pg && docker logs --tail 40 "$name" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
 $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export DOCKER_CONFIG=/srv/storage/butler/.cache/docker
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$name" --runtime runc --network "$network" --network-alias memory-postgres \
 --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges \
 --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=memory-fixture --env POSTGRES_DB=butler_memory_test \
 --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
 docker exec "$name" pg_isready -U fixture -d butler_memory_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
printf 'Memory candidate proof: %s\n' "$proof"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
 --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --env "BUTLER_MEMORY_CHECK_MODE=$check_mode" \
 --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:memory-fixture@memory-postgres:5432/butler_memory_test?sslmode=disable' \
 golang:1.27.2-bookworm bash -eu -c '
export BUTLER_GUARDED_REQUEST_BROKER_BINARY=/cache/go-tmp/memory-candidate-broker-$$.test
export BUTLER_MEMORY_CANDIDATE_APP_BINARY=/cache/go-tmp/memory-candidate-app-$$
trap "rm -f -- \"$BUTLER_GUARDED_REQUEST_BROKER_BINARY\" \"$BUTLER_MEMORY_CANDIDATE_APP_BINARY\"" EXIT
go test -race -c -o "$BUTLER_GUARDED_REQUEST_BROKER_BINARY" ./apps/tool-broker/internal/broker
go build -race -o "$BUTLER_MEMORY_CANDIDATE_APP_BINARY" ./apps/tool-broker
go test -json -race -p 1 -count=1 -run "^TestMemoryCandidate" ./internal/memory/candidatecontrol ./internal/config ./internal/actionauth ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/app
go test -json -race -p 1 -count=1 -run "^Test(Extraction|SourceDuplicate|ConcurrentExplicit)" ./internal/memory/catalog
go test -json -race -p 1 -count=1 -run "^TestExtractionJob" ./internal/memory/jobs
if [[ "$BUTLER_MEMORY_CHECK_MODE" == full ]]; then
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
go test -json -race -p 1 -count=1 ./internal/memory/catalog ./internal/memory/jobs ./internal/memory/candidatecontrol ./internal/config ./internal/actionauth ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/app ./apps/orchestrator/internal/orchestrator
fi
go vet ./internal/memory/catalog ./internal/memory/jobs ./internal/memory/candidatecontrol ./internal/config ./internal/actionauth ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/app ./apps/orchestrator/internal/app
' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl"
printf '{"status":"memory_candidate_passed","model_calls":0}\n'

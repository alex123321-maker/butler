#!/usr/bin/env bash
# Zero-inference integration proof for the combined optional voice/function code.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/internal/voice/store.go" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/optional-models-$run_id"
name="butler-optional-models-$run_id"
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
export DOCKER_CONFIG="$root/.cache/docker"
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$name" --network "$network" --network-alias optional-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=optional-fixture --env POSTGRES_DB=butler_optional_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_optional_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private optional integration proof: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:optional-fixture@optional-postgres:5432/butler_optional_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 -timeout=10m ./internal/audio ./internal/localmodels ./internal/functionproposal ./internal/voice ./apps/local-models ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app
go vet ./internal/audio ./internal/localmodels ./internal/functionproposal ./internal/voice ./apps/local-models ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
printf '{"status":"optional_integration_passed","model_calls":0,"actual_model_cases":"not_selected"}\n'

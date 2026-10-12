#!/usr/bin/env bash
# Isolated workspace-only PostgreSQL/filesystem proof; no shared Compose access.
set -euo pipefail
root=${1:?isolated agent root required}
mode=${2:-all}
[[ "$mode" == all || "$mode" == provenance || "$mode" == contracts || "$mode" == broker || "$mode" == bootstrap || "$mode" == ownership || "$mode" == recovery ]] || exit 2
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/migrations/060_isolated_code_workspaces.up.sql" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/workspace-$run_id"
name="butler-workspace-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$proof/socket" "$root/.cache/workspace-tmp"
if [[ "$mode" == contracts ]]; then
  export DOCKER_CONFIG="$root/.cache/docker"
  printf 'Offline workspace contract proof: %s\n' "$proof"
  docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none \
    --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
    --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
    golang:1.27.2-bookworm go test -json -race -count=1 \
      -run '^(TestWorkspaceContractsKeepOnlyDataAndFileCAS|TestWorkspaceGrantPinsOriginalInputAndUnavailableReadiness)$' \
      ./internal/workspaceexec ./internal/actionauth > "$proof/tests.jsonl"
  sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
  printf 'Workspace contracts and original-input admission tests passed.\n'
  exit 0
fi
created_network=false
created_pg=false
cleanup() {
  $created_pg && docker logs --tail 80 "$name" > "$proof/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export DOCKER_CONFIG="$root/.cache/docker"
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --runtime runc --name "$name" --network "$network" --network-alias workspace-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=workspace-fixture --env POSTGRES_DB=butler_workspace_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  # The image's temporary initialization server accepts Unix sockets before
  # creating POSTGRES_DB. Only the final server listens on TCP.
  if docker exec "$name" pg_isready -h 127.0.0.1 -U fixture -d butler_workspace_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
# Concurrent packages create independent schemas but share this disposable DB.
# Install its one extension before them; IF NOT EXISTS alone does not serialize
# simultaneous pg_extension inserts from separate package processes.
docker exec "$name" psql -q -v ON_ERROR_STOP=1 -U fixture -d butler_workspace_test \
  -c 'CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public' >/dev/null
printf 'Private workspace proof: %s\n' "$proof"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/workspace-tmp:/private-tmp" --workdir /src \
  --volume "$proof/socket:/run/butler-workspace" \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_POSTGRES_URL=postgres://fixture:workspace-fixture@workspace-postgres:5432/butler_workspace_test?sslmode=disable' \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:workspace-fixture@workspace-postgres:5432/butler_workspace_test?sslmode=disable' \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:workspace-fixture@workspace-postgres:5432/butler_workspace_test?sslmode=disable' \
  --env WORKSPACE_PROOF_MODE="$mode" \
  golang:1.27.2-bookworm bash -c '
    set -euo pipefail
    packages="./internal/workspaceexec"
    filter="."
    if [[ "$WORKSPACE_PROOF_MODE" == recovery ]]; then
      packages="./internal/workspace ./internal/workspaceexec ./apps/tool-workspace/..."
      filter="^TestWorkspaceOwnerDiscard|^TestWorkspaceRootOwner"
    fi
    gofmt -d internal/workspaceexec/recovery*.go internal/workspaceexec/copy_lane.go >/dev/null
    if [[ "$WORKSPACE_PROOF_MODE" == ownership ]]; then
      packages="./internal/workspace ./internal/workspaceexec"
      filter="^TestWorkspace(RootOwner|RuntimeClose|RuntimeRootReplacement|RuntimeRootBinding)"
    fi
    if [[ "$WORKSPACE_PROOF_MODE" == broker ]]; then
      packages="$packages ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/runtimeclient"
      filter="^(TestWorkspace|TestResourceAdmissions)"
    fi
    if [[ "$WORKSPACE_PROOF_MODE" == bootstrap || "$WORKSPACE_PROOF_MODE" == all ]]; then
      binary="/private-tmp/workspace-app-$$"
      service="/private-tmp/workspace-service-$$"
      trap '\''rm -f "$binary" "$service"'\'' EXIT
      CGO_ENABLED=0 go build -trimpath -o "$service" ./apps/tool-workspace
      go test -c -race -o "$binary" ./apps/tool-workspace/internal/app
      export BUTLER_WORKSPACE_E2E_BINARY="$binary"
    fi
    if [[ "$WORKSPACE_PROOF_MODE" == bootstrap ]]; then
      packages="$packages ./internal/config ./apps/tool-broker/internal/app ./apps/tool-workspace/... ./apps/orchestrator/internal/app"
      filter="^TestWorkspace"
    fi
    if [[ "$WORKSPACE_PROOF_MODE" == all ]]; then
      packages="$packages ./internal/workspace ./internal/memory/catalog ./internal/actionauth ./apps/tool-broker/... ./apps/tool-workspace/... ./internal/config"
    fi
    go test -json -race -count=1 -timeout=120s -run "$filter" $packages
    go vet $packages
    if [[ "$WORKSPACE_PROOF_MODE" == all ]]; then
      # Current owning input/readiness projections and their other control
      # callers are the affected orchestrator scope. Its full historical
      # converter/delegation suite is separately bounded and much larger.
      go test -json -race -count=1 -timeout=120s \
        -run "^(TestWorkspace|TestMemoryCandidate|TestGuardedTabRequest|TestApprovedCredentialBrokerBrowserE2E)" \
        ./apps/orchestrator/internal/app
      go vet ./apps/orchestrator/internal/app
    fi
  ' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
printf 'Workspace PG/filesystem race suite passed.\n'

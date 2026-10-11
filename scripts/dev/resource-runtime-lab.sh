#!/usr/bin/env bash
# Disposable resource/Broker/runtime PostgreSQL proof. No shared deployment.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/migrations/062_resource_dispatches.up.sql" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/resource-runtime-$run_id"
name="butler-resource-runtime-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$root/.cache/test-tmp"
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
docker run -d --name "$name" --network "$network" --network-alias resource-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=resource-fixture --env POSTGRES_DB=butler_resources_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_resources_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Isolated resource runtime proof: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/test-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:resource-fixture@resource-postgres:5432/butler_resources_test?sslmode=disable' \
  golang:1.27.2-bookworm go test -json -race -p=1 -count=1 -timeout=150s \
  -run '^TestResource|^TestRuntimePrivate|^TestAuthorityHTTP|^TestBrowserResource' \
  ./internal/resourceexec ./apps/tool-broker/internal/broker > "$proof/tests.jsonl"

# Broad regression runs only after the narrow new boundary checks pass. Each
# owning package uses disposable schemas; no shared laboratory database is used.
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/test-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:resource-fixture@resource-postgres:5432/butler_resources_test?sslmode=disable' \
  --env 'BUTLER_TEST_POSTGRES_URL=postgres://fixture:resource-fixture@resource-postgres:5432/butler_resources_test?sslmode=disable' \
  golang:1.27.2-bookworm go test -json -race -p=1 -count=1 -timeout=180s \
  ./internal/actionauth ./internal/budget ./internal/resources ./internal/resourceexec \
  ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/runtimeclient ./apps/tool-browser/internal/runtime \
  > "$proof/regression.jsonl"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 128 --cpus 2 \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache:/proof-cache" --workdir /src --env CGO_ENABLED=0 \
  --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  golang:1.27.2-bookworm go test -c -o /proof-cache/resource-browser.test ./apps/tool-browser/internal/runtime
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 128 --cpus 2 \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache:/proof-cache" --workdir /src --env CGO_ENABLED=0 \
  --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  golang:1.27.2-bookworm go test -c -o /proof-cache/resource-broker.test ./apps/tool-broker/internal/broker
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 256 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume "$root/.cache:/proof-cache:ro" \
  --volume "$source_dir/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" \
  --workdir /src/apps/tool-broker/internal/broker \
  --env BUTLER_BROWSER_E2E=1 --env BUTLER_BROWSER_E2E_BINARY=/proof-cache/resource-browser.test \
  --env BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:resource-fixture@resource-postgres:5432/butler_resources_test?sslmode=disable' \
  --entrypoint /proof-cache/resource-broker.test butler-lab-tool-browser \
  -test.v -test.run '^TestResourceBrokerActualChromiumPageCASIntegration$' > "$proof/chromium.log"
sha256sum "$proof/tests.jsonl" "$proof/regression.jsonl" "$proof/chromium.log" > "$proof/events.sha256"
printf 'Resource runtime PG/race suite passed.\n'

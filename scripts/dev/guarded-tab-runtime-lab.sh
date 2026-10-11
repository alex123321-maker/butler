#!/usr/bin/env bash
# Actual disposable PG/App bootstrap/private guarded runtime proof; zero inference.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
check_mode=${2:-narrow}
[[ "$check_mode" == narrow || "$check_mode" == full ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/guarded-runtime-$run_id"
name="butler-guarded-runtime-$run_id"
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
docker run -d --name "$name" --runtime runc --network "$network" --network-alias request-postgres \
 --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges \
 --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=request-fixture --env POSTGRES_DB=butler_request_test \
 --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
 docker exec "$name" pg_isready -U fixture -d butler_request_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
printf 'Private guarded runtime proof: %s\n' "$proof"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
 --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --env "BUTLER_RUNTIME_CHECK_MODE=$check_mode" \
 --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:request-fixture@request-postgres:5432/butler_request_test?sslmode=disable' \
 golang:1.27.2-bookworm bash -eu -c '
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
go test -json -race -p 1 -count=1 -run "^Test(SingleTabReadiness|SingleTabResource|GuardedTabAdmission|GuardedTabOrigin|GuardedTabRuntime)" ./internal/resourceexec ./internal/config ./apps/tool-broker/internal/app ./apps/tool-broker/internal/broker
if [[ "$BUTLER_RUNTIME_CHECK_MODE" == full ]]; then
go test -json -race -p 1 -count=1 ./internal/resourceexec ./internal/config ./internal/actionauth ./internal/singletabcontrol ./apps/tool-broker/internal/app ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/runtimeclient ./apps/browser-bridge/... ./apps/orchestrator/internal/orchestrator
fi
go vet ./internal/resourceexec ./internal/config ./internal/actionauth ./apps/tool-broker/internal/app ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/runtimeclient ./apps/browser-bridge/... ./apps/orchestrator/internal/app
' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl"
printf '{"status":"guarded_runtime_passed","model_calls":0}\n'

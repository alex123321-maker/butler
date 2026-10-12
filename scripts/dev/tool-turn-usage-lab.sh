#!/usr/bin/env bash
# Owning future usage regression: synthetic native SSE + real disposable PG.
# No provider auth/import, actual model requests, sealed windows or main services.
set -euo pipefail
root=${1:?isolated agent root required}
check_mode=${2:-all}
[[ "$check_mode" == all || "$check_mode" == narrow ]] || exit 2
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/orchestrator/budget_usage_integration_test.go" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/tool-turn-usage-$run_id"
name="butler-turn-usage-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" /srv/storage/butler/.cache/go-tmp
created_network=false
created_pg=false
cleanup() {
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export DOCKER_CONFIG="$root/.cache/docker"
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$name" --network "$network" --network-alias usage-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=usage-fixture --env POSTGRES_DB=butler_usage_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_usage_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private tool-turn usage proof: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/cache/go-tmp \
  --env "BUTLER_TURN_USAGE_CHECK_MODE=$check_mode" \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:usage-fixture@usage-postgres:5432/butler_usage_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c '
if [[ "$BUTLER_TURN_USAGE_CHECK_MODE" == all ]]; then
go test -json -race -p 1 -count=1 -timeout=90s ./internal/transport ./internal/transport/antigravity
else
go test -json -race -p 1 -count=1 -timeout=90s -run "^Test(ToolTurnUsage|CancelledIncompleteProviderTurn|CompleteInvalidToolProposal)" ./internal/transport/antigravity
fi
go test -json -race -p 1 -count=1 -timeout=180s -run "^TestModelTurnUsage" ./apps/orchestrator/internal/orchestrator
if [[ "$BUTLER_TURN_USAGE_CHECK_MODE" == all ]]; then
go test -json -race -p 1 -count=1 -timeout=90s -run "^Test(OwningSourceVerification|ExecuteRunsHappyPath)" ./apps/orchestrator/internal/orchestrator
fi
go vet ./internal/transport ./internal/transport/antigravity ./apps/orchestrator/internal/orchestrator' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
! grep -q '"Action":"skip"' "$proof/tests.jsonl"
printf '{"status":"tool_turn_usage_passed","model_calls":0,"historical_backfills":0}\n'

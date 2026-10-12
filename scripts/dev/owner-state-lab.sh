#!/usr/bin/env bash
# Disposable owning PG/HTTP and fake Telegram proof; no auth/provider/native use.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/task/owner_evidence.go" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/owner-state-$run_id"
name="butler-owner-state-$run_id"
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
docker run -d --name "$name" --network "$network" --network-alias owner-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=owner-fixture --env POSTGRES_DB=butler_owner_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_owner_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private owner state proof: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/cache/go-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:owner-fixture@owner-postgres:5432/butler_owner_test?sslmode=disable' \
  --env 'BUTLER_TEST_GOALS_POSTGRES_URL=postgres://fixture:owner-fixture@owner-postgres:5432/butler_owner_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 -timeout=90s -run "^Test(OwnerOverview|OwnerTask|OwnerMandateReference|OwnerApproval|OwnerWorkspace|Approvals|TaskRisk)" ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api ./apps/orchestrator/internal/goal ./apps/orchestrator/internal/approval
go test -json -race -p 1 -count=1 -timeout=90s ./apps/orchestrator/internal/channel/telegram
go test -json -race -p 1 -run "^$" ./apps/orchestrator/internal/app
go vet ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api ./apps/orchestrator/internal/goal ./apps/orchestrator/internal/approval ./apps/orchestrator/internal/channel/telegram ./apps/orchestrator/internal/app' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
if grep -q '"Action":"skip"' "$proof/tests.jsonl"; then exit 1; fi
printf '{"status":"owner_state_passed","model_calls":0,"real_human_messages":0}\n'

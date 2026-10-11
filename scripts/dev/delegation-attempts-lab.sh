#!/usr/bin/env bash
# Disposable PG and pool2/race proof; no inference or main credentials.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
check_mode=${2:-full}
[[ "$check_mode" == full || "$check_mode" == narrow || "$check_mode" == verification ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/delegation-attempts-$run_id"
name="butler-delegation-attempts-$run_id"
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
docker network create --internal "$network" > /dev/null
created_network=true
docker run -d --name "$name" --runtime runc --network "$network" --network-alias attempt-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=attempt-fixture --env POSTGRES_DB=butler_attempt_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB > /dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_attempt_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private failed-attempt proof: %s\n' "$proof"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
  --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env "BUTLER_ATTEMPT_CHECK_MODE=$check_mode" \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:attempt-fixture@attempt-postgres:5432/butler_attempt_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c '
if [[ "$BUTLER_ATTEMPT_CHECK_MODE" == verification ]]; then
go test -json -race -p 1 -count=1 -run "^TestDelegation(ConflictingReports|IndependentVerification|VerifiedPromotion)" ./apps/orchestrator/internal/app
go test -json -race -p 1 -count=1 -run "^TestDelegationOwnerDecision" ./apps/orchestrator/internal/api
go test -json -race -p 1 -count=1 ./internal/delegation
go test -json -race -p 1 -count=1 -run "^TestCapacityFailure" ./internal/transport/antigravity
go test -json -race -p 1 -count=1 -run "^TestFailedRunPersistsSafeTransportSubtype$" ./apps/orchestrator/internal/orchestrator
go test -json -race -p 1 -count=1 -run "^TestNativeDelegationOwnedAuditTransportDiagnosticsIntegration$" ./apps/orchestrator/internal/app
elif [[ "$BUTLER_ATTEMPT_CHECK_MODE" == narrow ]]; then
go test -json -race -p 1 -count=1 -run "^TestDelegationFailedAttempt" ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator
else
go test -json -race -p 1 -count=1 -run "^Test(Delegation|Delegated|Converter)" ./apps/orchestrator/internal/app
go test -json -race -p 1 -count=1 ./apps/orchestrator/internal/orchestrator
go test -json -race -p 1 -count=1 -run "^TestDelegation" ./apps/orchestrator/internal/task
go test -json -race -p 1 -count=1 ./internal/delegation
go test -json -race -p 1 -count=1 ./apps/orchestrator/internal/api ./internal/transport ./internal/transport/antigravity
go test -json -race -p 1 -count=1 -run "^TestNativeDelegationOwnedAuditTransportDiagnosticsIntegration$" ./apps/orchestrator/internal/app
fi
go vet ./apps/orchestrator/internal/task ./apps/orchestrator/internal/app ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./internal/delegation ./internal/transport/antigravity
' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl"
printf '{"status":"delegation_attempt_observations_passed","model_calls":0}\n'

#!/usr/bin/env bash
# Zero-inference private PG/Broker/core/worker diagnosis. No auth or native path.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/app/delegation_native_recovery_diagnosis_test.go" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/delegation-native-diagnosis-$run_id"
name="butler-native-diagnosis-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$root/.cache/delegation-tmp"
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
docker run -d --name "$name" --network "$network" --network-alias delegation-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=delegation-fixture --env POSTGRES_DB=butler_delegation_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_delegation_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private zero-inference diagnosis proof: %s\n' "$proof"
status=0
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/delegation-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c 'go test -race -c -o /private-tmp/diagnosis-broker.test ./apps/tool-broker/internal/broker
export BUTLER_DELEGATION_BROKER_BINARY=/private-tmp/diagnosis-broker.test
go test -json -race -run "^TestDelegatedInputVerifier" -count=1 -timeout=30s ./apps/orchestrator/internal/input
go test -json -race -run "^TestDelegation(Preparation|Recovery|Context|Accepted)" -count=1 -timeout=30s ./apps/orchestrator/internal/orchestrator
go test -json -race -run "^TestNativeDelegationZeroInferenceRecoveryDiagnosisIntegration$" -count=1 -timeout=120s ./apps/orchestrator/internal/app' > "$proof/tests.jsonl" || status=$?
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"diagnosis_passed","new_native_attempts":0,"historical_native_attempts":3,"total_native_attempts":3}\n'

#!/usr/bin/env bash
# Real private Broker/core/PG receipt proof; synthetic provider only, no auth.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/app/delegation_receipt_path_integration_test.go" ]] || exit 2
mode=${2:-narrow}
[[ "$mode" == narrow || "$mode" == full ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/delegation-receipts-$run_id"
name="butler-delegation-receipts-$run_id"
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
docker run -d --name "$name" --runtime runc --network "$network" --network-alias receipt-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=receipt-fixture --env POSTGRES_DB=butler_receipt_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_receipt_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private zero-inference receipt proof: %s\n' "$proof"
status=0
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
  --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/delegation-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env "BUTLER_RECEIPT_CHECK_MODE=$mode" \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:receipt-fixture@receipt-postgres:5432/butler_receipt_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c '
go test -race -c -o /private-tmp/receipt-broker.test ./apps/tool-broker/internal/broker
export BUTLER_DELEGATION_BROKER_BINARY=/private-tmp/receipt-broker.test
go test -json -race -p 1 -count=1 -run "^Test(ExecuteFinalBelongs|RecoveryDelegationReceipts)" ./apps/orchestrator/internal/orchestrator
go test -json -race -p 1 -count=1 -timeout=90s -run "^TestDelegationReceiptPath" ./apps/orchestrator/internal/app
go test -json -race -p 1 -count=1 -run "^TestFlashHigh.*Receipt" ./internal/transport/antigravity
if [[ "$BUTLER_RECEIPT_CHECK_MODE" == full ]]; then
go test -json -race -p 1 -count=1 ./apps/orchestrator/internal/orchestrator ./internal/transport/antigravity
go test -json -race -p 1 -count=1 -timeout=120s -run "^TestDelegationAdmission" ./apps/orchestrator/internal/app
fi
go vet ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app ./apps/tool-broker/internal/broker ./internal/transport/antigravity
' > "$proof/tests.jsonl" || status=$?
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"delegation_receipt_path_passed","model_calls":0}\n'

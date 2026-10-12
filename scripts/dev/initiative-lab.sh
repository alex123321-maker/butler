#!/usr/bin/env bash
set -euo pipefail
root=${1:?absolute isolated agent root}
mode=${2:-broad}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9-]+$ ]] || exit 2
[[ "$mode" == narrow || "$mode" == broad ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
stamp="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/$mode-$stamp"
pg="initiative-full-$stamp"
net="$pg-net"
umask 077
mkdir -p "$root/.cache/go-mod" "$root/.cache/go-race" "$root/.cache/go-tmp" "$root/.cache/docker" "$proof/postgres"
export DOCKER_CONFIG="$root/.cache/docker"
cleanup() { docker logs --tail 30 "$pg" > "$proof/postgres.log" 2>&1 || true; docker rm -f "$pg" >/dev/null 2>&1 || true; docker network rm "$net" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
docker network create --internal "$net" >/dev/null
docker run -d --name "$pg" --runtime runc --network "$net" --network-alias postgres --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=initiative-fixture --env POSTGRES_DB=butler_initiative_test --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
for i in $(seq 1 60); do docker exec "$pg" pg_isready -U fixture -d butler_initiative_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
printf 'Source %s; reports %s\n' "$source_dir" "$proof"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$net" --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges --volume "$source_dir:/src:ro" --volume "$root/.cache:/cache" --volume "$proof:/proof" --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env "MODE=$mode" --env BUTLER_DELEGATION_DIAGNOSTIC_REPORT_DIR=/proof --env BUTLER_TEST_INITIATIVE_POSTGRES_URL=postgres://fixture:initiative-fixture@postgres:5432/butler_initiative_test?sslmode=disable --env BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:initiative-fixture@postgres:5432/butler_initiative_test?sslmode=disable golang:1.27.2-bookworm bash -eu -c '
go build -race -p 1 -trimpath -o /proof/tool-broker ./apps/tool-broker
go test -c -race -p 1 -o /proof/tool-extension.test ./apps/tool-extension/internal/runtime
go test -c -race -p 1 -o /proof/orchestrator.test ./apps/orchestrator/internal/app
export BUTLER_EXTENSION_E2E=1
export BUTLER_BROKER_E2E_BINARY=/proof/tool-broker
export BUTLER_EXTENSION_E2E_BINARY=/proof/tool-extension.test
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:initiative-fixture@postgres:5432/butler_initiative_test?sslmode=disable
go tool test2json -t -p github.com/butler/butler/apps/orchestrator/internal/app /proof/orchestrator.test -test.v -test.timeout=180s -test.run "^TestInitiativeProject(FinalProjection|ChecksRealBrokerE2E)$" > /proof/events.jsonl
if [[ "$MODE" == broad ]]; then
go test -json -race -p 1 -count=1 ./apps/orchestrator/internal/initiative >> /proof/events.jsonl
fi
go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/initiative > /proof/vet.log 2>&1
'
if grep -E '"Action":"(skip|fail)"' "$proof/events.jsonl" >/dev/null; then printf 'Unexpected skip/fail retained\n'; exit 1; fi
printf 'Pass events: '; grep -cE '"Action":"pass","Package".*"Test":' "$proof/events.jsonl"
sha256sum "$proof/events.jsonl" "$proof/vet.log"
printf 'Inference0; race/vet complete; no skips\n'

#!/usr/bin/env bash
# Fourteen-pin accounting and real PG contention; no authentication/inference.
set -euo pipefail
root="$1"
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/app/initiative_high_window_test.go" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/initiative-high-window-$run_id"
name="butler-initiative-high-window-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$root/.cache/initiative-high-tmp"
export DOCKER_CONFIG="$root/.cache/docker"
created_network=false
created_pg=false
cleanup() {
  $created_pg && docker logs --tail 40 "$name" > "$proof/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$name" --runtime runc --network "$network" --network-alias initiative-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=initiative-fixture --env POSTGRES_DB=butler_initiative_window_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$name" pg_isready -U fixture -d butler_initiative_window_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Zero-inference initiative window evidence: %s\n' "$proof"
(cd "$source_dir"; find . -type f -print0 | sort -z | xargs -0 sha256sum) > "$proof/source.sha256"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
  --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/initiative-high-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:initiative-fixture@initiative-postgres:5432/butler_initiative_window_test?sslmode=disable' \
  golang:1.27.2-bookworm go test -json -race -p 1 -count=1 -timeout=90s \
  -run '^TestInitiativeHigh(History|LimitIntegration|RevalidationIntegration|PreservesSpentDelegationWindowIntegration|ExpiryAfterRowWaitIntegration)$' \
  ./apps/orchestrator/internal/app > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
! grep -q '"Action":"skip"' "$proof/tests.jsonl" || exit 1
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none \
  --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  golang:1.27.2-bookworm go vet ./apps/orchestrator/internal/app > "$proof/vet.txt"
printf '{"status":"initiative_high_window_preparation_passed","native_calls":0}\n'

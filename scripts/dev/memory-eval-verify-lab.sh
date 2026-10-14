#!/usr/bin/env bash
# Component regression with fake inference, isolated PostgreSQL, zero model calls.
set -euo pipefail
root=${1:?isolated agent root}
checks=${2:-full}
[[ "$checks" == full || "$checks" == narrow ]] || exit 2
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
tag="memory78-fixture-$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/$tag"
mkdir -p "$proof/postgres" "$root/.cache/go-race" "$root/.cache/go-tmp" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
docker network create --internal "$tag" > "$proof/network-id"
docker run -d --name "$tag-pg" --network "$tag" --network-alias postgres --cpus 1 --memory 512m --pids-limit 128 \
 --security-opt no-new-privileges:true --mount "type=bind,src=$proof/postgres,dst=/var/lib/postgresql/data" \
 --env POSTGRES_HOST_AUTH_METHOD=trust --env POSTGRES_USER=butler --env POSTGRES_DB=memory_eval_test pgvector/pgvector:pg17 > "$proof/postgres-id"
trap 'docker stop "$tag-pg" >/dev/null; docker network rm "$tag" >/dev/null || true' EXIT
for _ in $(seq 1 30); do if docker exec "$tag-pg" pg_isready -U butler -d memory_eval_test >/dev/null; then break; fi; sleep 1; done
docker run --rm --network "$tag" --cpus 2 --memory 1536m --pids-limit 128 --security-opt no-new-privileges:true \
 -v "$source_dir:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" \
 -w /src -e GOCACHE=/cache/go-race -e GOMODCACHE=/gomod -e GOTMPDIR=/cache/go-tmp \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgresql://butler@postgres:5432/memory_eval_test?sslmode=disable' golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 -run "^TestMemory(V2|EvalCollector)" ./internal/acceptance ./apps/orchestrator/internal/app > /proof/narrow.jsonl
if [[ "$1" == full ]]; then go test -json -race -p 1 -count=1 ./internal/acceptance ./cmd/memory-eval ./internal/transport/... > /proof/regression.jsonl; fi
go vet ./internal/acceptance ./cmd/memory-eval ./internal/transport/... ./apps/orchestrator/internal/app
' checks "$checks" > "$proof/runner.log" 2> "$proof/diagnostics.log"
printf 'Fixture proof: %s\n' "$proof"

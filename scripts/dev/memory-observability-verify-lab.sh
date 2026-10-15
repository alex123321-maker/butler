#!/usr/bin/env bash
set -euo pipefail
base=${1:?isolated source root}
[[ "$base" =~ ^/srv/storage/butler/agents/memory-observability-[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
tag=memory-observability-$(date -u +%Y%m%dT%H%M%SZ)-$$
proof=$base/reports/$tag
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
docker network create --internal --subnet "${BUTLER_VERIFY_SUBNET:-10.253.81.0/28}" "$tag" > "$proof/network-id"
trap 'docker rm -f "$tag-pg" >/dev/null 2>&1 || true; docker network rm "$tag" >/dev/null 2>&1 || true' EXIT
docker run -d --name "$tag-pg" --network "$tag" --network-alias postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_observability_test pgvector/pgvector:pg17 > "$proof/postgres-id"
for _ in $(seq 1 30); do if docker exec "$tag-pg" pg_isready -U butler -d memory_observability_test >/dev/null; then break; fi; sleep 1; done
printf 'Memory observability verification: %s\n' "$proof"
printf '%s\n' "$src" > "$proof/source-path"
docker run --rm --network "$tag" --cpus 2 --memory 1536m --pids-limit 128 \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_observability_test?sslmode=disable' \
 -e BUTLER_TEST_SEMANTIC_POSTGRES_URL='postgres://butler@postgres:5432/memory_observability_test?sslmode=disable' \
 -e BUTLER_TEST_DOCTOR_BROKER_BINARY=/proof/tool-broker golang:1.27.2-bookworm bash -eu -c '
go build -o /proof/tool-broker ./apps/tool-broker
go test -json -race -p 1 -count=1 ./internal/memory/diagnostics ./internal/memory/doctor ./internal/memory/service ./apps/orchestrator/internal/api ./apps/orchestrator/internal/input > /proof/unit-storage.jsonl
go test -json -race -p 1 -count=1 ./internal/memory/semantic ./internal/metrics > /proof/semantic-metrics.jsonl
go test -json -race -p 1 -count=1 -timeout 3m -run "^TestOperatorDoctorActualBrokerIntegration$" ./apps/orchestrator/internal/app > /proof/broker-composition.jsonl
go test -json -race -p 1 -count=1 ./apps/orchestrator/internal/orchestrator > /proof/orchestrator.jsonl
go vet ./internal/memory/diagnostics ./internal/memory/doctor ./internal/memory/service ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app
' > "$proof/runner.log" 2> "$proof/diagnostics.log"
printf 'PASS\n' > "$proof/complete"
jq -s '{pass:map(select(.Action=="pass" and .Test!=null))|length,fail:map(select(.Action=="fail"))|length,skip:map(select(.Action=="skip"))|length}' "$proof/"*.jsonl

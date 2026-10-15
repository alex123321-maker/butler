#!/usr/bin/env bash
set -euo pipefail
base=${1:?isolated source root}
mode=${2:-all}
[[ "$mode" == all || "$mode" == foundation ]] || exit 2
[[ "$base" =~ ^/srv/storage/butler/agents/memory-search-[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
tag=memory-search-verify-$(date -u +%Y%m%dT%H%M%SZ)-$$
proof=$base/reports/$tag
umask 077
mkdir -p "$proof/postgres"
docker network create --internal "$tag" > "$proof/network-id"
docker run -d --name "$tag-pg" --network "$tag" --network-alias postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_search_test pgvector/pgvector:pg17 > "$proof/postgres-id"
trap 'docker stop "$tag-pg" >/dev/null || true' EXIT
for _ in $(seq 1 30); do if docker exec "$tag-pg" pg_isready -U butler -d memory_search_test >/dev/null; then break; fi; sleep 1; done
printf 'Search/context verification: %s\n' "$proof"
printf '%s\n' "$src" > "$proof/source-path"
docker run --rm --network "$tag" --cpus 2 --memory 1536m --pids-limit 128 \
 -v "$src:/src:ro" -v /srv/storage/butler/agents/memory-eval-78-20261014/.cache:/cache \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-timeout -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_search_test?sslmode=disable' \
 -e BUTLER_TEST_SEMANTIC_POSTGRES_URL='postgres://butler@postgres:5432/memory_search_test?sslmode=disable' -e VERIFY_MODE="$mode" golang:1.27.2-bookworm bash -eu -c '
if [[ "$VERIFY_MODE" == foundation ]]; then
 go test -json -race -p 1 -count=1 -run "^TestHybrid" ./internal/memory/semantic > /proof/foundation.jsonl
 exit 0
fi
go test -json -race -p 1 -count=1 ./internal/memory/service > /proof/service.jsonl
go test -json -race -p 1 -count=1 -timeout 10m -run "^Test(LocalOnly|CurrentLocalOnly|OwnedSourceSnapshot|Hybrid|Admission|KnownCredential|BackendToken|Fusion|Technical|Distinct|Admitted)" ./internal/memory/semantic > /proof/semantic.jsonl
go test -json -race -p 1 -count=1 -run "^Test(AssignedLocalOnly|OptionalContext|ContextBudget|SummaryCannot|CoveredSummary|TranscriptReplay|ExecuteSessionSummaryWithProfile|ExecuteUsesEmbedding|ExecuteIncludesSessionSummary)" ./apps/orchestrator/internal/orchestrator > /proof/orchestrator.jsonl
go test -json -race -p 1 -count=1 -timeout 10m -run "^Test(CurrentProjectRule|MemorySearch)" ./apps/orchestrator/internal/app > /proof/app.jsonl
go vet ./internal/memory/service ./internal/memory/semantic ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app
' > "$proof/runner.log" 2> "$proof/diagnostics.log"
printf 'PASS\n' > "$proof/complete"
jq -s '{pass:map(select(.Action=="pass" and .Test!=null))|length,fail:map(select(.Action=="fail"))|length,skip:map(select(.Action=="skip"))|length}' "$proof/"*.jsonl

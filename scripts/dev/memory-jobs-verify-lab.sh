#!/usr/bin/env bash
set -euo pipefail
base=${1:?isolated source root}
label=${2:?fresh evidence label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue82-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
export ISSUE82_ROOT=$base
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof"
compose=(docker compose -p butler-issue82 -f "$base/current/deploy/memory-jobs/compose.yml")
"${compose[@]}" up -d postgres redis
for db in memory_jobs_storage_test memory_jobs_legacy_test;do
 if ! docker exec butler-issue82-postgres-1 psql -U butler -d postgres -Atc 'SELECT datname FROM pg_database' | grep -qx "$db";then docker exec butler-issue82-postgres-1 createdb -U butler "$db";fi
done
"${compose[@]}" run --rm verify sh -eu -c "
go build -o /proof/tool-broker ./apps/tool-broker
go test -json -race -p 1 -count=1 -timeout 3m ./internal/memory/jobs ./internal/budget ./internal/actionauth ./internal/memory/doctor ./internal/transport/antigravity > /proof/$label/narrow.jsonl
BUTLER_TEST_POSTGRES_URL='postgres://butler@postgres:5432/memory_jobs_legacy_test?sslmode=disable' go test -json -race -p 1 -count=1 -timeout 5m ./internal/memory/catalog ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/completion > /proof/$label/services.jsonl
go test -json -race -p 1 -count=1 -timeout 3m -run '^TestOperatorDoctorActualBrokerIntegration$' ./apps/orchestrator/internal/app > /proof/$label/doctor-broker.jsonl
go vet ./internal/memory/jobs ./internal/budget ./internal/memory/doctor ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app
go build -o /proof/orchestrator ./apps/orchestrator
" > "$proof/runner.log" 2>&1
jq -s '{pass:map(select(.Action=="pass" and .Test!=null))|length,fail:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip")|{package:.Package,test:.Test})}' "$proof/"*.jsonl > "$proof/summary.json"
printf 'PASS\n' > "$proof/complete"
sha256sum "$base/reports/orchestrator" "$base/reports/tool-broker" > "$proof/binaries.sha256"
cat "$proof/summary.json"

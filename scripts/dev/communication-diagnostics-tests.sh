#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
export GOTOOLCHAIN=local
export GOCACHE="$PWD/.cache/go-build-linux"
export GOMODCACHE="${GOMODCACHE:-$PWD/.cache/go-mod}"
export GOFLAGS=-buildvcs=false
export BUTLER_TEST_COMMUNICATION_POSTGRES_URL="${BUTLER_TEST_COMMUNICATION_POSTGRES_URL:-postgres://postgres@127.0.0.1:55446/communication_diagnostics_test?sslmode=disable}"
export BUTLER_TEST_MEMORY_POSTGRES_URL="$BUTLER_TEST_COMMUNICATION_POSTGRES_URL"
mkdir -p .cache/communication-diagnostics
report=.cache/communication-diagnostics/targeted.jsonl
if ! go test -p 1 -race -count=1 -json ./internal/communicationcontrol ./apps/orchestrator/internal/communication ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app -run '^TestCommunication(Diagnostic|OwnerQuestionLimit|OwnerProfile)' >"$report"; then
 grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$report" || tail -50 "$report"
 exit 1
fi
if grep -q '"Action":"skip"' "$report"; then grep '"Action":"skip"' "$report"; exit 1; fi
regression=.cache/communication-diagnostics/regression.jsonl
if ! go test -p 1 -race -count=1 -json ./apps/orchestrator/internal/communication ./apps/orchestrator/internal/app ./apps/orchestrator/internal/input -run '^TestCommunication(OrderedMigration|RollbackRefuses|WidgetMigration|EpochAnd|NoExpiry|FastResponse|UnknownDelivery|CorrectionIs|NewOrRebound|SourceDeletion|AncestorDeletion|DeniedExport|ActualACKPersistence|ActualCoreFreshContext|TaskPause|TaskAmend|StopAmend|MediaPolicy|MediaMigration|DrainRetains|InputClaimCancellation)' >"$regression"; then
 grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$regression" || tail -50 "$regression"
 exit 1
fi
if grep -q '"Action":"skip"' "$regression"; then grep '"Action":"skip"' "$regression"; exit 1; fi
go vet ./internal/communicationcontrol ./apps/orchestrator/internal/communication ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app
echo 'Communication diagnostics PostgreSQL/race tests and vet passed without skips.'

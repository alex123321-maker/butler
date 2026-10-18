#!/usr/bin/env bash
set -euo pipefail
cd /src
export GOTOOLCHAIN=local
export GOCACHE=/src/.cache/go-build-linux
export GOMODCACHE=/src/.cache/go-mod-linux
export BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/butler_schedules_test?sslmode=disable'
export BUTLER_TEST_INITIATIVE_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_SCHEDULES_APP_BINARY=/tmp/schedules-broker
export BUTLER_GUARDED_REQUEST_BROKER_BINARY="$BUTLER_SCHEDULES_APP_BINARY"
export BUTLER_MEMORY_CANDIDATE_APP_BINARY="$BUTLER_SCHEDULES_APP_BINARY"
mkdir -p .cache/schedules
go build -race -o "$BUTLER_SCHEDULES_APP_BINARY" ./apps/tool-broker
report=.cache/schedules/targeted.jsonl
if ! go test -race -count=1 -json ./internal/scheduling ./internal/schedulecontrol ./internal/actionauth ./internal/config ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/scheduled ./apps/orchestrator/internal/app -run '^(TestSchedule|TestSchedules|TestOneShot)' >"$report"; then
 grep -E '"Action":"fail"|"OutputType":"error"' "$report" || tail -40 "$report"
 exit 1
fi
if grep -q '"Action":"skip"' "$report"; then
 grep '"Action":"skip"' "$report"
 echo 'A mandatory schedule test was skipped' >&2
 exit 1
fi
go test -race -count=1 ./internal/actionauth ./internal/budget ./internal/config ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/initiative
go test -race -count=1 ./apps/orchestrator/internal/app -run '^Test(DelegationAdmissionTokenIsNotAuthorityIntegration|MemoryCandidateDispatchedIntegration)$'
go vet ./internal/scheduling ./internal/schedulecontrol ./internal/actionauth ./internal/budget ./internal/config ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/scheduled ./apps/orchestrator/internal/app
echo 'Schedule integration, ordinary execution, delivery, race and compatibility checks passed.'

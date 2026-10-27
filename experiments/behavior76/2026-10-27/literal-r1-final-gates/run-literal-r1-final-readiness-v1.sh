#!/bin/sh
set -u
cd /src20
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2
export GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly'
go vet ./apps/orchestrator/internal/app > .cache/behavior76/literal-r1-final-vet-v1.log 2>&1
vet_status=$?
printf '{"version":"literal-r1-final-vet-v1","exit":%s,"source":"c8411429e36f2e7d085971617f0f84dc2f2e5b6e","native_calls":0}\n' "$vet_status" > .cache/behavior76/literal-r1-final-vet-v1-exit.json
if [ "$vet_status" -ne 0 ]; then exit 1; fi
/usr/bin/psql -X -v ON_ERROR_STOP=1 -h 127.0.0.1 -p 55451 -U postgres -d behavior76_test -f /src20/.cache/behavior76/check-closed-local-response-v10-r3-history-v1.sql > .cache/behavior76/literal-r1-current-history-v1.log 2>&1
history_status=$?
printf '{"version":"literal-r1-current-history-v1","exit":%s,"source":"c8411429e36f2e7d085971617f0f84dc2f2e5b6e","closed_windows":12,"admissions":971,"measured":959,"known_full_tokens":7700211,"NULL_usage":12,"application_journal_writes":0,"temporary_client_staging":true,"actual_comparison":"BEGIN READ ONLY then ROLLBACK","native_calls":0}\n' "$history_status" > .cache/behavior76/literal-r1-current-history-v1-exit.json
if [ "$history_status" -ne 0 ]; then exit 1; fi
export BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only
export BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable'
export BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker
export BUTLER_BEHAVIOR76_SOURCE='git:c8411429e36f2e7d085971617f0f84dc2f2e5b6e;binary-sha256:7336e3fcfc282f9663a7fb23641146400900a7056e8a57889b803d272c3c411e'
/src20/.cache/behavior76/literal-collector-c8411429-r1.test -test.v -test.timeout=90s -test.run='^TestBehavior76LocalUnknownResourceProfileIntegration$' > .cache/behavior76/literal-r1-current-resource-v1.log 2>&1
resource_status=$?
printf '{"version":"literal-r1-current-resource-v1","exit":%s,"elf_sha256":"7336e3fcfc282f9663a7fb23641146400900a7056e8a57889b803d272c3c411e","native_calls":0}\n' "$resource_status" > .cache/behavior76/literal-r1-current-resource-v1-exit.json
exit "$resource_status"
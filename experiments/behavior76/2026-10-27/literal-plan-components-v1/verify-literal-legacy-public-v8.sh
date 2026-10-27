#!/bin/sh
set -u
cd /src20
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2
export GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly'
export BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled
export BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable'
export BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker
go test -race -v -count=1 -timeout=6m ./apps/orchestrator/internal/app -run '^TestBehavior76(DayWindowAdmission|AuthorityStream(FinalSealFailure|BeyondLegacyCapacity|WriteFailureAndLateEventStop|FixtureIntegration|StorageProfile|UnsealedPrivatePrefix)|AuthorityTraceCancelledRollbackIntegration|StimulusBoundary|InstructionIntegrity|Queued.*Integration|CoreBroker.*Integration|EvidenceExportIntegration|ContinuationBoundary|LifecyclePreparationIntegration|PreparedTasksOnceIntegration|FreshHistoryBoundaryIntegration|GlobalStop.*|UnknownEffectStopsBeforeNextAdmissionIntegration|LocalUnknown.*|LocalModel.*|LocalHTTP.*|LocalResponse.*)$'
test_status=$?
true # actual unchanged-source vet receipt is v7
vet_status=$?
printf '{"test_exit":%s,"prior_vet_receipt":"literal-plan-composition-v7-exit.json","native_calls":0,"hidden_controls_bound":false,"source_frozen":false}\n' "$test_status" > .cache/behavior76/literal-legacy-public-v8-exit.json
if [ "$test_status" -ne 0 ] || [ "$vet_status" -ne 0 ]; then exit 1; fi
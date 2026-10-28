#!/bin/sh
set -u
cd /src20
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2
export GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly'
export BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled
export BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable'
export BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker
go test -race -v -count=1 -timeout=180s ./apps/orchestrator/internal/app -run '^TestBehavior76Literal'
test_status=$?
go vet ./apps/orchestrator/internal/app
vet_status=$?
printf '{"test_exit":%s,"vet_exit":%s,"native_calls":0,"hidden_controls_bound":false,"source_frozen":false}\n' "$test_status" "$vet_status" > .cache/behavior76/literal-observation-v5-exit.json
if [ "$test_status" -ne 0 ] || [ "$vet_status" -ne 0 ]; then exit 1; fi
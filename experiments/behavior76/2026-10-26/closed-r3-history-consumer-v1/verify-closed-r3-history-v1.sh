#!/bin/sh
set -eu
cd /src20
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2
export GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly'
export BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only
export BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled
export BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable'
go test -race -v -count=1 -timeout=180s ./apps/orchestrator/internal/app -run '^TestBehavior76FreshHistoryBoundaryIntegration$'
go vet ./apps/orchestrator/internal/app

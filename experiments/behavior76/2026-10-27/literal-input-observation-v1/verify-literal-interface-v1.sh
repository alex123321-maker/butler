#!/bin/sh
set -eu
cd /src20
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2
export GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly'
export BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled
go test -race -v -count=1 -timeout=90s ./apps/orchestrator/internal/app -run '^TestBehavior76LiteralDeclaredPlanAndRawOwnerInterface$'
go vet ./apps/orchestrator/internal/app

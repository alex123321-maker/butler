#!/bin/sh
set -eu
cd /src21
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 GOFLAGS=-buildvcs=false
export DOCKER_HOST=unix:///run/butler21-docker.sock DOCKER_CONFIG=/src21/.cache/docker-private21
test -z "$(docker ps -q)"
export BUTLER_TEST_MAINTENANCE_GUARD_KERNEL=private-trusted-guard-only
export BUTLER_TEST_MAINTENANCE_GUARD_IMAGE=5a7e4124365ea4f271c23e1c9c918b991f86d675e0f2789e520f87a321062165
export BUTLER_TEST_MAINTENANCE_GUARD_SHA256=c62270e6ca372fa6062f41247bdcfcb9a34d3649cf7df0e58a3119ed16add1c9
export BUTLER_TEST_MAINTENANCE_CANDIDATE_SHA256=a8ce3e6c649def3681f4959a334e88d700b3b10fd1ecea18383057b719800296
go test -race -v -count=1 -timeout=90s ./internal/maintenance -run '^TestCapsuleGuard(ExactEnvironmentAndMountBoundary|ColdIdentityIsNotRuntime|OwnedKernelIntegration)$'
go vet ./internal/maintenance
test -z "$(docker ps -q)"

#!/bin/sh
set -eu
cd /src21
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 GOFLAGS=-buildvcs=false
export DOCKER_HOST=unix:///run/butler21-docker.sock DOCKER_CONFIG=/src21/.cache/docker-private21
test "$(cat /sys/fs/cgroup/butler-maintenance21/memory.swap.max)" = 0
test -z "$(docker ps -q)"
export BUTLER_TEST_MAINTENANCE_BUILD=private-local-build-only
export BUTLER_TEST_MAINTENANCE_GUARDED_PACKAGE=private-local-no-execution
export BUTLER_TEST_MAINTENANCE_COMPILER_SHA256=0a761904b78c29eba4ec83bd83b7312db1e113dcd92da7fc669843ad9bca56a1
export BUTLER_TEST_MAINTENANCE_GUARD_ELF=/src21/.cache/maintenance21-guard-process-v1/root/maintenance-guard
export BUTLER_TEST_MAINTENANCE_GUARD_SHA256=2b7d42c682ad98dc78ec9bfce442b8d4a7f0e25b601623d1b922e48c025340d3
go test -race -v -count=1 -timeout=300s ./internal/maintenance -run '^Test(DockerBuilderOwnedDaemonIntegration|DockerGuardedPackagerOwnedDaemonIntegration|GuardedArchiveRequiresExactRegularMember)$'
go vet ./internal/maintenance ./cmd/maintenance-capsule-guard
test -z "$(docker ps -q)"

#!/bin/sh
# Prepared only. Run after the separately admitted issue76 collection has exited.
set -eu
cd /src21
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin
export GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache
export GOTOOLCHAIN=local GOFLAGS=-buildvcs=false GOMAXPROCS=2
export DOCKER_HOST=unix:///run/butler21-docker.sock
export DOCKER_CONFIG=/src21/.cache/docker-private21
test "$(cat /sys/fs/cgroup/butler-maintenance21/memory.swap.max)" = 0
test -z "$(docker ps --format '{{.ID}}')"
test "$(/usr/bin/psql -h 127.0.0.1 -p 55451 -U postgres -d behavior76_test -At -v ON_ERROR_STOP=1 -c 'SELECT count(*) FROM public.behavior76_windows WHERE NOT closed')" = 0
compiler=$(docker image inspect golang:1.27.2-alpine --format '{{.Id}}')
case "$compiler" in sha256:????????????????????????????????????????????????????????????????) ;; *) exit 2;; esac
export BUTLER_TEST_MAINTENANCE_BUILD=private-local-build-only
export BUTLER_TEST_MAINTENANCE_COMPILER_SHA256=${compiler#sha256:}
export BUTLER_TEST_MAINTENANCE_DOCKER=private-local-inspection-only
export BUTLER_TEST_MAINTENANCE_DOCKER_IMAGE=30ecbe1509090b38ef3260f3609d64d8261230464eaa4d235901db496e9647fb
gofmt -w internal/maintenance/docker_builder.go internal/maintenance/docker_artifact.go internal/maintenance/docker_builder_test.go internal/maintenance/build_bounds.go internal/maintenance/docker_build_bounds_test.go internal/maintenance/docker_build_output_unix_test.go internal/maintenance/node_observer.go internal/maintenance/node_observer_test.go internal/maintenance/node_observer_unix_test.go apps/maintenance-node/main.go apps/maintenance-node/main_integration_test.go
go test -race -v -count=1 -timeout=240s ./internal/maintenance ./apps/maintenance-node -run '^Test(Docker(Build.*|Package.*)|NodeObserver.*|NodeObservationChallenge.*|MaintenanceNode.*)$'
go vet ./internal/maintenance ./apps/maintenance-node

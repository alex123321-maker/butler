#!/bin/sh
# Requires explicitly isolated local PG, Broker binary and network-none runtime.
# No SSH, Docker Desktop daemon mutation or provider call in the offline mode.
set -eu
cd "$(dirname "$0")/../.."
action=${1:-offline}
case "$action" in offline|native) ;; *) echo 'Use offline or native' >&2; exit 2;; esac
: "${BUTLER_CONVERTER_BROKER_BINARY:?actual local production Broker binary required}"
: "${BUTLER_CONVERTER_PROFILE_JSON:?pinned actual runtime profile required}"
: "${BUTLER_CONVERTER_RUNTIME_TOKEN_FILE:?isolated synthetic token file required}"
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?isolated local test database required}"
export BUTLER_CONVERTER_DEVELOPER_E2E=1
export GOTOOLCHAIN=local
export GOFLAGS=-buildvcs=false
export GOMAXPROCS=4
export GOCACHE="${GOCACHE:-$PWD/.cache/go-build-linux}"
export GOMODCACHE="${GOMODCACHE:-$PWD/.cache/go-mod}"
mkdir -p .cache/converter20
binary="$PWD/.cache/converter20/orchestrator-app.test"
go test -race -c -o "$binary" ./apps/orchestrator/internal/app
if [ "$action" = native ]; then
 test "${BUTLER_CONVERTER_LOCAL_NATIVE:-}" = synthetic-local-only
 changes=$(git status --porcelain) || exit 1
 test -z "$changes" || { echo 'Commit prospective native source first' >&2; exit 1; }
 commit=$(git rev-parse --verify HEAD) || exit 1
 export BUTLER_CONVERTER_LOCAL_NATIVE_SOURCE="git:$commit;binary-sha256:$(sha256sum "$binary" | cut -d' ' -f1)"
 "$binary" -test.v -test.timeout=23m -test.run '^TestConverterDeveloperActualHighLocalContainerE2E$'
else
 "$binary" -test.v -test.timeout=8m -test.run '^TestConverter(CompoundSourceProofDoesNotBorrowAuthorityReserveIntegration|DeveloperBoundedLocalContainerE2E|DeveloperGenericFactoryLocalContainerE2E)$'
 go vet ./apps/orchestrator/internal/app
fi

#!/bin/sh
# Requires explicitly isolated local PG, Broker binary and network-none runtime.
# No SSH, Docker Desktop daemon mutation or provider call in the offline mode.
set -eu
cd "$(dirname "$0")/../.."
action=${1:-offline}
case "$action" in offline|streaming|native) ;; *) echo 'Use offline, streaming or native' >&2; exit 2;; esac
: "${BUTLER_CONVERTER_BROKER_BINARY:?actual local production Broker binary required}"
: "${BUTLER_CONVERTER_PROFILE_JSON:?pinned actual runtime profile required}"
: "${BUTLER_CONVERTER_RUNTIME_TOKEN_FILE:?isolated synthetic token file required}"
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?isolated local test database required}"
export BUTLER_CONVERTER_DEVELOPER_E2E=1
export BUTLER_TEST_COMMUNICATION_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export GOTOOLCHAIN=local
export GOFLAGS=-buildvcs=false
export GOMAXPROCS=4
export GOCACHE="${GOCACHE:-$PWD/.cache/go-build-linux}"
export GOMODCACHE="${GOMODCACHE:-$PWD/.cache/go-mod}"
mkdir -p .cache/converter20
binary="$PWD/.cache/converter20/orchestrator-app.test"
go test -race -c -o "$binary" ./apps/orchestrator/internal/app
# Keep canonical binaries/caches in the repository, but execute identical bytes
# on the local Linux filesystem. Windows-backed ELF page faults are outside the
# provider/authority proof and must not consume its unchanged two-second bound.
execution_dir=$(mktemp -d /tmp/butler-converter20.XXXXXX)
trap 'rm -f "$execution_dir/app.test" "$execution_dir/tool-broker"; rmdir "$execution_dir"' EXIT
cp "$binary" "$execution_dir/app.test"
cp "$BUTLER_CONVERTER_BROKER_BINARY" "$execution_dir/tool-broker"
cmp "$binary" "$execution_dir/app.test"
cmp "$BUTLER_CONVERTER_BROKER_BINARY" "$execution_dir/tool-broker"
export BUTLER_CONVERTER_BROKER_BINARY="$execution_dir/tool-broker"
if [ "$action" = native ]; then
 test "${BUTLER_CONVERTER_LOCAL_NATIVE:-}" = synthetic-local-only
 "$execution_dir/app.test" -test.v -test.timeout=1m -test.run '^TestConverterLocalNativeReservationsCoverModelAndToolQuotesIntegration$'
 digest=$(sha256sum "$binary" | cut -d' ' -f1)
 if [ -n "${BUTLER_CONVERTER_LOCAL_NATIVE_SOURCE:-}" ]; then
  # The Windows wrapper verified the exact managed checkout after building.
  test "${BUTLER_CONVERTER_LOCAL_NATIVE_SOURCE##*;binary-sha256:}" = "$digest"
 else
  changes=$(git status --porcelain) || exit 1
  test -z "$changes" || { echo 'Commit prospective native source first' >&2; exit 1; }
  commit=$(git rev-parse --verify HEAD) || exit 1
  export BUTLER_CONVERTER_LOCAL_NATIVE_SOURCE="git:$commit;binary-sha256:$digest"
 fi
 "$execution_dir/app.test" -test.v -test.timeout=23m -test.run '^TestConverterDeveloperActualHighLocalContainerE2E$'
elif [ "$action" = streaming ]; then
 "$execution_dir/app.test" -test.v -test.timeout=6m -test.run '^TestConverterDeveloperStreamingLocalContainerE2E$'
 go vet ./apps/orchestrator/internal/app
else
 "$execution_dir/app.test" -test.v -test.timeout=8m -test.run '^(TestConverter(CompoundSourceProofDoesNotBorrowAuthorityReserveIntegration|DeveloperGenericFactoryLocalContainerE2E|LocalNativeReservationsCoverModelAndToolQuotesIntegration)|TestCommunicationAuthorityReserveWhileBusinessPoolSaturatedIntegration)$'
 go vet ./apps/orchestrator/internal/app
fi

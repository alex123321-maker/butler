#!/bin/sh
# Synthetic local-only race/vet gate. No inference, auth import or real tools.
set -eu
cd "$(dirname "$0")/../.."
export GOTOOLCHAIN=local
export GOMAXPROCS=4
export GOFLAGS=-buildvcs=false
export GOCACHE="$PWD/.cache/go-build-linux"
export GOMODCACHE="${GOMODCACHE:-$PWD/.cache/go-mod}"
export BUTLER_EFFORT_LAB_POSTGRES_URL='postgres://postgres@127.0.0.1:55449/effort97_test?sslmode=disable'
mkdir -p .cache/effort97
report=.cache/effort97/offline-linux-race.jsonl
# Two unrelated acceptance fixtures need the full memory DB or an old private
# failed-request file. They are explicitly unselected, not counted as passed.
if ! go test -p 1 -race -count=1 -json ./cmd/effort-eval ./internal/acceptance/effort ./internal/transport/antigravity ./internal/transport -skip 'TestMemoryJobActualNativeAdapterHTTPIntegration|TestTemporalFailedV1RequestPreHTTPDiagnostic' > "$report"; then
 tail -40 "$report"; exit 1
fi
if grep -q '"Action":"skip"' "$report"; then grep '"Action":"skip"' "$report"; exit 1; fi
go vet ./cmd/effort-eval ./internal/acceptance/effort ./internal/transport/antigravity ./internal/transport
echo 'Local synthetic effort race/PG/wire tests and vet passed without skips.'

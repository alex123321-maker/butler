#!/usr/bin/env bash
set -euo pipefail
cd /src
export GOTOOLCHAIN=local
export GOCACHE=/src/.cache/go-build-linux
export GOMODCACHE=/src/.cache/go-mod-linux
export BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/butler_tool_discovery_test?sslmode=disable'
export BUTLER_TEST_INITIATIVE_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_MEMORY_POSTGRES_URL"
export BUTLER_TOOL_DISCOVERY_APP_BINARY=/tmp/tool-discovery-broker
export BUTLER_GUARDED_REQUEST_BROKER_BINARY="$BUTLER_TOOL_DISCOVERY_APP_BINARY"
mkdir -p .cache/tool-discovery
go build -race -o "$BUTLER_TOOL_DISCOVERY_APP_BINARY" ./apps/tool-broker
report=".cache/tool-discovery/targeted-$(date -u +%Y%m%dT%H%M%SZ)-$$.jsonl"
packages=(./internal/toolcatalog ./internal/transport/... ./internal/actionauth ./internal/config ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/input ./apps/orchestrator/internal/app)
if ! go test -race -count=1 -json "${packages[@]}" -run '^(TestTool(Discovery|Catalog|Search|Set|SchemaProjection)|TestActionSlot)' >"$report"; then
  grep -E '"Action":"fail"|"OutputType":"error"' "$report" || tail -50 "$report"
  exit 1
fi
if grep -q '"Action":"skip"' "$report"; then
  grep '"Action":"skip"' "$report"
  echo 'A mandatory discovery test was skipped' >&2
  exit 1
fi
go vet "${packages[@]}"
cp "$report" .cache/tool-discovery/targeted.jsonl
echo 'Discovery PostgreSQL/process, model-loop, wire-contract and race checks passed. No native model inference was performed.'

#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
export GOTOOLCHAIN=local
export GOCACHE="$PWD/.cache/go-build-linux"
export GOMODCACHE="$PWD/.cache/go-mod"
export BUTLER_TEST_COMMUNICATION_POSTGRES_URL="${BUTLER_TEST_COMMUNICATION_POSTGRES_URL:-postgres://postgres@127.0.0.1:5432/butler_communication_linux_test?sslmode=disable}"
export BUTLER_TEST_MEMORY_POSTGRES_URL="$BUTLER_TEST_COMMUNICATION_POSTGRES_URL"
export BUTLER_TEST_INITIATIVE_POSTGRES_URL="$BUTLER_TEST_COMMUNICATION_POSTGRES_URL"
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="$BUTLER_TEST_COMMUNICATION_POSTGRES_URL"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_COMMUNICATION_POSTGRES_URL"
export BUTLER_TEST_COMPLETION_POSTGRES_URL="$BUTLER_TEST_COMMUNICATION_POSTGRES_URL"
mkdir -p .cache/communication-core
report=.cache/communication-core/targeted.jsonl
packages=(./internal/communicationcontrol ./internal/actionauth ./internal/budget ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/communication ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/input ./apps/orchestrator/internal/app ./internal/memory/compaction ./internal/memory/pipeline)
if ! go test -p 1 -race -count=1 -json "${packages[@]}" -run '^TestCommunication' >"$report"; then
 grep -E '"Action":"fail"|"OutputType":"error"' "$report" || tail -40 "$report"
 exit 1
fi
if grep -q '"Action":"skip"' "$report"; then
 grep '"Action":"skip"' "$report"
 exit 1
fi
go vet "${packages[@]}" ./apps/orchestrator/internal/input ./apps/orchestrator/internal/completion ./apps/orchestrator/internal/task ./apps/orchestrator/internal/run ./apps/orchestrator/internal/deliveryoutbox ./internal/memory/transcript
if [[ ${1:-} == --regression ]]; then
 regression=(./apps/orchestrator/internal/input ./apps/orchestrator/internal/completion ./apps/orchestrator/internal/task ./apps/orchestrator/internal/run ./apps/orchestrator/internal/deliveryoutbox ./apps/orchestrator/internal/orchestrator ./apps/tool-broker/internal/broker ./internal/actionauth ./internal/budget ./internal/memory/transcript ./internal/memory/compaction ./internal/memory/pipeline)
 if ! go test -p 1 -count=1 -json "${regression[@]}" -skip '(Native|Backup|ServerBrowser)' >.cache/communication-core/regression.jsonl; then
  grep -E '"Action":"fail"|"OutputType":"error"' .cache/communication-core/regression.jsonl || true
  exit 1
 fi
 # A separately selected shared private-control fixture changed in this slice.
 go test -c -race -o .cache/communication-core/broker.test ./apps/tool-broker/internal/broker
 export BUTLER_DELEGATION_BROKER_BINARY="$PWD/.cache/communication-core/broker.test"
 if ! go test -p 1 -race -count=1 -json ./apps/orchestrator/internal/app -run '^TestDelegationAdmission' >.cache/communication-core/control-regression.jsonl; then
  grep -E '"Action":"fail"|"OutputType":"error"' .cache/communication-core/control-regression.jsonl || true
  exit 1
 fi
 if grep -q '"Action":"skip"' .cache/communication-core/control-regression.jsonl; then
  grep '"Action":"skip"' .cache/communication-core/control-regression.jsonl; exit 1
 fi
 go build -o .cache/communication-core/orchestrator ./apps/orchestrator
 go build -o .cache/communication-core/tool-broker ./apps/tool-broker
fi
echo 'Communication kernel: targeted PostgreSQL/race checks passed without skips; vet passed. No model inference or channel send.'

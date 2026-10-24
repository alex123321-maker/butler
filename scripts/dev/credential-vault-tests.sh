#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${BUTLER_TEST_VAULT_POSTGRES_URL:?Disposable *_test PostgreSQL database required}"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_VAULT_POSTGRES_URL"
export GOTOOLCHAIN=local
mkdir -p .cache/credential-vault-ci

required() {
 local report=".cache/credential-vault-ci/$1.jsonl"
 shift
 if ! go test -p 1 -race -count=1 -json "$@" >"$report"; then
  grep -E '"Action":"fail"|"OutputType":"error"|"Output":".*[.]go:[0-9]+' "$report" || tail -40 "$report"
  exit 1
 fi
 if grep -q '"Action":"skip"' "$report"; then
  echo 'Required credential case skipped'; grep '"Action":"skip"' "$report"; exit 1
 fi
 if ! grep -q '"Action":"pass","Package".*"Test"' "$report"; then
  echo 'No required test executed'; exit 1
 fi
}

# Full ordered production migrations, including pgvector and 096, not just fixtures.
required migrations ./internal/credentials -run '^TestStoreIntegrationCRUD$'
required vault ./internal/credentials ./internal/actionauth ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/channel/telegramrich ./apps/tool-broker/internal/broker -run '^Test(Vault|Credential)'
go test -c -race -o .cache/credential-vault-ci/browser.test ./apps/tool-browser/internal/runtime
export BUTLER_BROWSER_E2E=1
export BUTLER_BROWSER_E2E_BINARY="$PWD/.cache/credential-vault-ci/browser.test"
export BUTLER_BROWSER_E2E_SCRIPT="$PWD/apps/tool-browser/scripts/browser_runtime.mjs"
required broker-chromium ./apps/tool-broker/internal/broker -run '^TestBrokerBrowserSequenceE2E$'
required runtime-chromium ./apps/tool-browser/internal/runtime -run '^TestBrowserSessionE2E$'
go vet ./internal/credentials ./internal/credentialcontrol ./internal/actionauth ./internal/mobile ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app ./apps/tool-broker/internal/broker ./apps/tool-browser/internal/runtime

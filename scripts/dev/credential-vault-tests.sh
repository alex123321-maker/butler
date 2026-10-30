#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${BUTLER_TEST_VAULT_POSTGRES_URL:?Disposable *_test PostgreSQL database required}"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_VAULT_POSTGRES_URL"
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="$BUTLER_TEST_VAULT_POSTGRES_URL"
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
required vault ./internal/budget ./internal/credentials ./internal/actionauth ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/channel/telegramrich ./apps/tool-broker/internal/broker -run '^Test(Vault|Credential)'
required owner-outcome-resume ./apps/orchestrator/internal/app -run '^TestBrowserOwnerOutcomeSameRunRecoveryIntegration$'
go test -c -race -o .cache/credential-vault-ci/browser.test ./apps/tool-browser/internal/runtime
export BUTLER_BROWSER_E2E=1
export BUTLER_BROWSER_E2E_BINARY="$PWD/.cache/credential-vault-ci/browser.test"
export BUTLER_BROWSER_E2E_SCRIPT="$PWD/apps/tool-browser/scripts/browser_runtime.mjs"
required broker-chromium ./apps/tool-broker/internal/broker -run '^TestBrokerBrowserSequenceE2E$'
required browser-safety ./apps/tool-broker/internal/broker -run '^TestBrowserSafety(CurrentAuthorityAndBinding|BrokerProcessRestartKeepsUnknown|RuntimeLoss)ChromiumIntegration$'
for case in \
 TestBrowserSafetyCurrentAuthorityAndBindingChromiumIntegration/unchanged_control \
 TestBrowserSafetyCurrentAuthorityAndBindingChromiumIntegration/task_cancel \
 TestBrowserSafetyCurrentAuthorityAndBindingChromiumIntegration/mandate_revoke \
 TestBrowserSafetyCurrentAuthorityAndBindingChromiumIntegration/changed_origin \
 TestBrowserSafetyCurrentAuthorityAndBindingChromiumIntegration/changed_args \
 TestBrowserSafetyBrokerProcessRestartKeepsUnknownChromiumIntegration/after_opaque_ack \
 TestBrowserSafetyBrokerProcessRestartKeepsUnknownChromiumIntegration/during_lost_ack \
 TestBrowserSafetyRuntimeLossChromiumIntegration/fresh_context_required \
 TestBrowserSafetyRuntimeLossChromiumIntegration/before_dispatch \
 TestBrowserSafetyRuntimeLossChromiumIntegration/after_recipient_post; do
 if ! grep -F '"Action":"pass"' .cache/credential-vault-ci/browser-safety.jsonl | grep -Fq "\"Test\":\"$case\""; then
  printf 'Required browser safety case absent: %s\n' "$case"; exit 1
 fi
done
required browser-resource-fixture ./apps/tool-broker/internal/broker -run '^TestResource.*Integration$'
required browser-session-recovery ./apps/tool-broker/internal/broker -run '^TestBrowserSessionRecoveryAfterRuntimeLossChromiumIntegration$'
required browser-frozen-navigation ./apps/tool-broker/internal/broker -run '^TestBrowserFrozenNavigationRecoveryChromiumIntegration$'
required browser-form-validation ./apps/tool-broker/internal/broker -run '^TestBrowserFormValidationChromiumIntegration$'
required runtime-form-validation ./apps/tool-browser/internal/runtime -run '^TestBrowserPublicFormValidationChromium$'
required scoped-login-pg ./internal/actionauth -run '^TestCredentialLoginPostgresEvidenceAndNoReplayIntegration$'
required scoped-login-runtime ./apps/tool-browser/internal/runtime -run '^TestBrowserLoginChromium$'
required scoped-login-contract ./internal/browserauth ./internal/actionauth ./internal/research -run '^TestLogin|^TestCredentialLogin|^TestResearchVerifiedLogin'
go test -c -race -o .cache/credential-vault-ci/owner-api.test ./apps/orchestrator/internal/api
export BUTLER_BROWSER_OWNER_API_BINARY="$PWD/.cache/credential-vault-ci/owner-api.test"
export BUTLER_TEST_MEMORY_POSTGRES_URL="$BUTLER_TEST_VAULT_POSTGRES_URL"
export BUTLER_BROWSER_MANDATE_E2E=1
required browser-owner-mandate ./apps/tool-broker/internal/broker -run '^TestBrowserOwnerMandateApprovalReductionChromiumIntegration$'
required runtime-chromium ./apps/tool-browser/internal/runtime -run '^TestBrowserSessionE2E$'
go vet ./internal/budget ./internal/credentials ./internal/credentialcontrol ./internal/actionauth ./internal/mobile ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app ./apps/tool-broker/internal/broker ./apps/tool-browser/internal/runtime

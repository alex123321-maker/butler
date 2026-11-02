#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${BUTLER_TEST_VAULT_POSTGRES_URL:?Disposable PostgreSQL required}"
export GOTOOLCHAIN=local
report=.cache/browser-sso-execution-ci
mkdir -p "$report"
required() {
 local name="$1"; shift
 printf 'stage=%s started_at=%s\n' "$name" "$(date -u +%FT%TZ)"
 if ! go test -p 1 -race -count=1 -json "$@" >"$report/$name.jsonl"; then
  grep -E '"Action":"fail"|"OutputType":"error"|"Output":".*[.]go:[0-9]+' "$report/$name.jsonl" || tail -40 "$report/$name.jsonl"
  exit 1
 fi
 if grep -q '"Action":"skip"' "$report/$name.jsonl"; then echo 'Required SSO case skipped';exit 1;fi
 grep -F '"Action":"pass"' "$report/$name.jsonl" | grep -Fq '"Test":'
 printf 'stage=%s finished_at=%s\n' "$name" "$(date -u +%FT%TZ)"
}
required migrations ./internal/credentials -run '^TestStoreIntegrationCRUD$'
required authority ./internal/browsersso ./internal/actionauth -run '^TestSSO'
required historical-recovery ./internal/actionauth -run '^TestOwnerOutcomeHistoricalLost(SSO|Login)'
required consent-use ./internal/credentials ./apps/orchestrator/internal/api -run '^TestVaultSSO|^TestCredentialVaultOwnerAPIPrivacyLocal$'
export BUTLER_TEST_BROWSER_PAGES_POSTGRES_URL="$BUTLER_TEST_VAULT_POSTGRES_URL"
required dispatch-fence ./internal/browsercontrol -run '^TestSSOPreDispatchPageFenceIntegration$'
required same-input ./apps/orchestrator/internal/app -run '^TestCredentialVaultLocalWaitResumeAndExpiry$'
required recipient-fault-controls ./cmd/browser-sso-eval -run '^TestSSOFixtureCancelledAcceptedLoginRetainsReceiptWithoutCallback$'
go build -o "$report/recipient" ./cmd/browser-sso-eval
export BUTLER_SSO_EVAL_BINARY="$PWD/$report/recipient"
export BUTLER_BROWSER_E2E_SCRIPT="$PWD/apps/tool-browser/scripts/browser_runtime.mjs"
export BUTLER_BROWSER_SSO_E2E=1
required https-runtime ./apps/tool-browser/internal/runtime -run '^TestBrowserSSOChromiumHTTPS$'
for mode in verified late_popup wrong_password denied_idp_post two_popups popup_storm stale_page helper_lost_before_sso helper_lost_after_idp_login; do
 grep -F '"Action":"pass"' "$report/https-runtime.jsonl" | grep -Fq "\"Test\":\"TestBrowserSSOChromiumHTTPS/$mode\""
done
# These are mechanical checks with no Butler model invocation. Their durations
# never substitute for native ingress-to-delivery timing or Google acceptance.

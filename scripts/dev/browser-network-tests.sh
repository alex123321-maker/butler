#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
export GOTOOLCHAIN=local
export GOCACHE="$PWD/.cache/go-build-linux"
export GOMODCACHE="$PWD/.cache/go-mod"
export NPM_CONFIG_CACHE="$PWD/.cache/npm"
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$PWD/.cache/ms-playwright}"
reports="$PWD/.cache/browser-network-ci"
mkdir -p "$reports"

required() {
  local report="$reports/$1.jsonl"
  shift
  if ! go test -race -count=1 -json "$@" >"$report"; then
    tail -50 "$report"; exit 1
  fi
  if grep -Fq '"Action":"skip"' "$report"; then
    echo 'Required browser network check skipped'; exit 1
  fi
}
passed() {
  if ! grep -F '"Action":"pass"' "$reports/$1.jsonl" | grep -Fq "\"Test\":\"$2\""; then
    printf 'Required browser network case absent: %s\n' "$2"; exit 1
  fi
}
required proxy ./internal/security/egress
for name in TestProxySSEFlushAndFreeze TestProxyHTTPResponseAllowanceIsCumulativeAndTerminal TestProxyHTTPAndCONNECTShareResponseAllowance TestProxyOutboundAllowanceExhaustionIsTerminal TestProxyConcurrentAdmissionIsBoundedBeforeDNS TestProxyReconnectAllowanceSurvivesFreezeAndThaw TestScopedProxyFreezeClosesEstablishedCONNECT; do
  passed proxy "$name"
done
required spa-contract ./internal/browserspa/... ./cmd/browser-spa-eval
required spa-authority ./internal/actionauth -run '^TestSPA'
required spa-bootstrap ./apps/tool-broker/internal/app ./apps/tool-broker/internal/broker -run '^TestSPA(Composition|Admission)'
required spa-raw-runtime ./apps/tool-browser/internal/runtime -run '^TestSPARawRuntimeCannotBypassResourceWrapper$'
test -n "${BUTLER_TEST_ACTIONAUTH_POSTGRES_URL:-}"
required spa-postgres ./apps/tool-broker/internal/broker -run '^TestSPABrokerRunningAuthorityAndNoReplayIntegration$'
for mode in allowed path_denied foreign_origin cancelled revoked constraint_changed source_removed input_replaced input_expired lost_finish expired; do
  passed spa-postgres "TestSPABrokerRunningAuthorityAndNoReplayIntegration/$mode"
done
go vet ./internal/security/egress ./internal/security/spaproxy ./internal/browserspa/... ./internal/resourceexec ./apps/tool-browser/internal/runtime ./cmd/browser-spa-eval
export BUTLER_BROWSER_E2E=1
export BUTLER_BROWSER_E2E_SCRIPT="$PWD/apps/tool-browser/scripts/browser_runtime.mjs"
required spa-relay ./internal/security/spaproxy
passed spa-relay TestRelayActualChromiumWorkerPOSTWSSSSE
passed spa-relay TestChromiumWorkerRestartCacheAndTerminalFreeze
required spa-runtime ./apps/tool-browser/internal/runtime -run '^TestSPAReadChromium$'
for mode in normal cookie_reflection mutation private foreign worker_mutation worker_private ws_send oversized storm lost_query infinite cancelled revoked helper_lost; do
  passed spa-runtime "TestSPAReadChromium/$mode"
done
required chromium ./apps/tool-browser/internal/runtime -run '^TestBrowserSessionE2E$'
passed chromium TestBrowserSessionE2E
passed chromium 'TestBrowserSessionE2E/live_SSE_stops_at_preparation_freeze_and_automatic_reconnect_cannot_thaw'
passed chromium 'TestBrowserSessionE2E/operator_domain_cap_denies_foreign_resources_POST_private_origins_and_WebSocket'
passed chromium 'TestBrowserSessionE2E/local_preparation_freezes_immediate_and_delayed_autosave_through_observations_and_stale_clicks'
echo 'SPA PG authority, proxy race/vet and actual Chromium network regressions passed; zero native model calls.'

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
go vet ./internal/security/egress ./apps/tool-browser/internal/runtime
export BUTLER_BROWSER_E2E=1
required chromium ./apps/tool-browser/internal/runtime -run '^TestBrowserSessionE2E$'
passed chromium TestBrowserSessionE2E
passed chromium 'TestBrowserSessionE2E/live_SSE_stops_at_preparation_freeze_and_automatic_reconnect_cannot_thaw'
passed chromium 'TestBrowserSessionE2E/operator_domain_cap_denies_foreign_resources_POST_private_origins_and_WebSocket'
passed chromium 'TestBrowserSessionE2E/local_preparation_freezes_immediate_and_delayed_autosave_through_observations_and_stale_clicks'
echo 'Proxy race/vet and actual Chromium network regressions passed; zero native model calls.'

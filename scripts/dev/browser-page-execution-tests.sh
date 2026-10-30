#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${BUTLER_TEST_ACTIONAUTH_POSTGRES_URL:?Disposable *_test PostgreSQL required}"
root="$PWD"
report="$root/.cache/browser-page-execution-ci/linux-$(date -u +%Y%m%dT%H%M%S)-$$"
mkdir -p "$report"
export BUTLER_TEST_POSTGRES_URL="$BUTLER_TEST_ACTIONAUTH_POSTGRES_URL"
export BUTLER_TEST_BROWSER_PAGES_POSTGRES_URL="$BUTLER_TEST_ACTIONAUTH_POSTGRES_URL"
required() {
 local name="$1"; shift
 if ! go test -race -count=1 -json "$@" >"$report/$name.jsonl"; then tail -60 "$report/$name.jsonl"; exit 1; fi
 if grep -q '"Action":"skip"' "$report/$name.jsonl"; then echo "Required $name skipped"; exit 1; fi
 grep -F '"Action":"pass"' "$report/$name.jsonl" | grep -Fq '"Test":'
}
# These profiles are mandatory, not availability-dependent component skips.
required migrations ./internal/credentials -run '^TestStoreIntegrationCRUD$'
required page-authority ./internal/actionauth -run '^TestPage'
required contracts ./internal/browserpage
required catalog ./apps/tool-broker/internal/broker -run '^TestPageCatalog'
required ledger ./internal/browsercontrol -run '^TestPage'
go vet ./internal/browserpage ./internal/actionauth ./internal/browsercontrol ./internal/resourceexec ./apps/tool-browser/internal/runtime ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/prompt
export BUTLER_BROWSER_PAGES_E2E=1
export BUTLER_BROWSER_E2E_SCRIPT="$root/apps/tool-browser/scripts/browser_runtime.mjs"
required runtime ./apps/tool-browser/internal/runtime -run '^TestManagedPagesChromium$'
for case in same_title_exact_target foreign_first_request_denied outside_gesture_denied owner_selection_and_global_fence late_popup_and_live_cap close_exact_page_retains_identity runtime_loss_no_restore; do
 grep -F '"Action":"pass"' "$report/runtime.jsonl" | grep -Fq "\"Test\":\"TestManagedPagesChromium/$case\""
done
go test -c -race -o "$report/browser.test" ./apps/tool-browser/internal/runtime
go test -c -race -o "$report/api.test" ./apps/orchestrator/internal/api
go test -c -race -o "$report/broker.test" ./apps/tool-broker/internal/broker
export BUTLER_BROWSER_E2E_BINARY="$report/browser.test"
export BUTLER_SERVER_BROWSER_API_BINARY="$report/api.test"
export BUTLER_BROWSER_PAGES_BROKER_E2E=1
export BUTLER_MANAGED_PAGES_UI_ADDR=127.0.0.1:18164
export BUTLER_MANAGED_PAGES_REAL_API=http://127.0.0.1:18164
export BUTLER_MANAGED_PAGES_UI_READY="$report/ready"
export BUTLER_MANAGED_PAGES_UI_DONE="$report/done"
export BUTLER_MANAGED_PAGES_UI_ABORT="$report/abort"
export BUTLER_WEB_TEST_PORT=18165 NITRO_PORT=18165 NITRO_HOST=127.0.0.1
npm run --prefix web build >"$report/web-build.log" 2>&1
preview=''; backend=''
cleanup() {
 touch "$report/abort"
 if [[ -n "$backend" ]] && kill -0 "$backend" 2>/dev/null; then
  for _ in {1..50}; do kill -0 "$backend" 2>/dev/null || break; sleep .1; done
  kill "$backend" 2>/dev/null || true
 fi
 if [[ -n "$preview" ]]; then kill "$preview" 2>/dev/null || true; fi
}
trap cleanup EXIT
(cd web && exec node .output/server/index.mjs) >"$report/web-preview.log" 2>&1 & preview=$!
for _ in {1..100}; do curl -fsS http://127.0.0.1:18165/ >/dev/null 2>&1 && break; sleep .2; done
curl -fsS http://127.0.0.1:18165/ >/dev/null
(cd apps/tool-broker/internal/broker && exec go tool test2json -t -p broker-managed-pages "$report/broker.test" -test.v '-test.run=^TestManagedPagesBrokerChromiumIntegration$' -test.timeout=150s) >"$report/broker.jsonl" 2>"$report/broker-error.log" & backend=$!
for _ in {1..300}; do [[ -e "$report/ready" ]] && break; kill -0 "$backend"; sleep .1; done
test -f "$report/ready"
(cd web && npx playwright test tests/e2e/server-browser-pages-real.spec.ts --workers=1 --reporter=json --output "$report/web-real-results") >"$report/web-real.json"
wait "$backend"; backend=''
! grep -Eq '"Action":"(skip|fail)"' "$report/broker.jsonl"
grep -F '"Action":"pass"' "$report/broker.jsonl" | grep -Fq '"Test":"TestManagedPagesBrokerChromiumIntegration"'
(cd web && npx playwright test tests/e2e/server-browser.spec.ts tests/e2e/server-browser-pages.spec.ts --workers=1 --reporter=json --output "$report/web-component-results") >"$report/web-component.json"
echo "Actual owner UI/Core/Broker/PG/resource/Chromium PASS; model_calls=0; evidence=$report"

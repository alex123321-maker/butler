#!/usr/bin/env bash
# Requires disposable *_test PG, browser runtime binary, Web deps and Chromium.
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?Disposable *_test PostgreSQL required}"
: "${BUTLER_BROWSER_E2E_BINARY:?Actual isolated browser test binary required}"
export BUTLER_BROWSER_OWNER_UI_E2E=1
export BUTLER_BROWSER_E2E=1
export BUTLER_BROWSER_OWNER_API_BINARY="$PWD/.cache/credential-vault-ci/owner-api.test"
proof="$PWD/.cache/credential-vault-ci"
mkdir -p "$proof"
go test -c -race -o "$BUTLER_BROWSER_OWNER_API_BINARY" ./apps/orchestrator/internal/api
go test -c -race -o "$proof/owner-broker.test" ./apps/tool-broker/internal/broker
(cd web && npm run build) > "$proof/owner-web-build.log" 2>&1
PORT="${BUTLER_WEB_TEST_PORT:-4173}" HOST=127.0.0.1 node web/.output/server/index.mjs > "$proof/owner-web-server.log" 2>&1 &
web_pid=$!
broker_pid=''
cleanup() { if [[ -n "$broker_pid" ]]; then kill "$broker_pid" >/dev/null 2>&1 || true; fi; kill "$web_pid" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
node -e 'const url="http://127.0.0.1:"+(process.env.BUTLER_WEB_TEST_PORT||4173); (async()=>{for(let i=0;i<100;i++){try{if((await fetch(url)).ok)return}catch{} await new Promise(r=>setTimeout(r,100))}process.exit(1)})()'
"$proof/owner-broker.test" -test.v -test.timeout=6m -test.run '^TestBrowserSafetyOwnerUIChromiumIntegration$' > "$proof/owner-broker.log" 2>&1 &
broker_pid=$!
if ! (cd web && npx --no-install playwright test --workers=1 tests/e2e/browser-owner-real.spec.ts) > "$proof/owner-browser.log" 2>&1; then
 cat "$proof/owner-browser.log" "$proof/owner-broker.log"
 exit 1
fi
if ! wait "$broker_pid"; then cat "$proof/owner-broker.log"; exit 1; fi
! grep -q -- '--- SKIP:' "$proof/owner-broker.log"
for kind in stop revoke; do
 grep -q "owner_ui=$kind real_owner_receipts=1 recipient_posts=0 original_grant=authorized original_dispatches=0" "$proof/owner-broker.log"
done
grep -q 'owner_ui=unchanged_control recipient_posts=1 opaque_effect=effect_unknown' "$proof/owner-broker.log"
grep -q '1 passed' "$proof/owner-browser.log"
cat "$proof/owner-browser.log"

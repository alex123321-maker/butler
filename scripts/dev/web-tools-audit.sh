#!/usr/bin/env bash
# Read-only product audit in an already synced, isolated agent root.
# probes deliberately runs regression expectations that may FAIL; never call
# those failures successful acceptance. No shared Compose or owner credentials.
set -euo pipefail
root=${1:?isolated agent root required}
mode=${2:?unit|browser|policy|probes|web required}
[[ "$root" =~ ^/srv/storage/butler/agents/web-tools-audit-[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
mkdir -p "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/reports"
export DOCKER_CONFIG="$root/.cache/docker"
mkdir -p "$DOCKER_CONFIG"
# Serialize compilation and temporary fixture injection inside this audit root.
exec 9>"$root/.cache/audit.lock"
flock 9
report="$root/reports/$mode-$(date -u +%Y%m%dT%H%M%SZ).log"
exec > >(tee "$report") 2>&1
printf 'mode=%s source=%s report=%s\n' "$mode" "$src" "$report"
go_run() {
  docker run --rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 \
    --security-opt no-new-privileges:true -v "$src:/src:ro" -v "$root/.cache:/cache" \
    -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src \
    -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off \
    -e GOTMPDIR=/cache/go-tmp -e CGO_ENABLED=0 golang:1.27.2-alpine "$@"
}
case "$mode" in
  unit)
    go_run go test -json -p 2 -count=1 ./apps/tool-http/... ./apps/tool-webfetch/... \
      ./apps/tool-browser/... ./apps/tool-browser-local/... ./apps/tool-broker/internal/registry \
      ./apps/tool-broker/internal/broker ./internal/security/egress/... ./internal/singletabcontrol
    ;;
  browser)
    go_run go test -c -o /cache/browser-runtime.test ./apps/tool-browser/internal/runtime
    go_run go test -c -o /cache/browser-broker.test ./apps/tool-broker/internal/broker
    for suite in runtime broker; do
      test_name='^TestBrowserSessionE2E$'
      [[ "$suite" == broker ]] && test_name='^TestBrokerBrowserSequenceE2E$'
      docker run --rm --user "$(id -u):$(id -g)" --memory 2g --cpus 2 --pids-limit 256 \
        --security-opt no-new-privileges:true --shm-size 256m \
        -v "$root/.cache:/cache" -v "$src/configs:/app/configs:ro" \
        -v "$src/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" -w /app \
        -e BUTLER_BROWSER_E2E=1 -e BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs \
        -e BUTLER_BROWSER_E2E_BINARY=/cache/browser-runtime.test \
        --entrypoint "/cache/browser-$suite.test" butler-lab-tool-browser \
        -test.v -test.timeout=180s -test.run "$test_name"
    done
    ;;
  policy)
    go_run go test -c -o /cache/browser-runtime.test ./apps/tool-browser/internal/runtime
    go_run go test -c -o /cache/browser-broker.test ./apps/tool-broker/internal/broker
    go_run go build -o /cache/guarded-native-host ./apps/browser-bridge/cmd/guarded
    pg="butler-web-audit-pg-$(date -u +%Y%m%dT%H%M%SZ)-$$"
    mkdir -p "$root/.cache/$pg"
    created=false
    cleanup() { if $created; then docker rm -f "$pg" >/dev/null; fi; }
    trap cleanup EXIT
    docker run -d --name "$pg" --network none --memory 768m --cpus 1 --pids-limit 128 \
      -v "$root/.cache/$pg:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust \
      -e POSTGRES_DB=web_audit_test pgvector/pgvector:pg17 -c max_connections=40 >/dev/null
    created=true
    for i in $(seq 1 30); do
      docker exec "$pg" pg_isready -U postgres -d web_audit_test >/dev/null 2>&1 && break
      sleep 1
    done
    docker exec "$pg" pg_isready -U postgres -d web_audit_test
    dsn='postgres://postgres@127.0.0.1:5432/web_audit_test?sslmode=disable'
    docker run --rm --user "$(id -u):$(id -g)" --network "container:$pg" \
      --memory 3g --cpus 2 --pids-limit 256 -v "$src:/src:ro" -v "$root/.cache:/cache" \
      -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src \
      -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off \
      -e GOTMPDIR=/cache/go-tmp -e CGO_ENABLED=0 -e "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=$dsn" \
      golang:1.27.2-alpine go test -json -p 1 -count=1 -timeout=180s \
      -run '^TestBrowserLocalPreparation|^TestNetworkRelease|^TestSingleTab|^TestResource' \
      ./internal/actionauth ./apps/tool-broker/internal/broker ./internal/singletabcontrol
    docker run --rm --user "$(id -u):$(id -g)" --network "container:$pg" \
      --memory 2g --cpus 2 --pids-limit 256 --shm-size 256m --security-opt no-new-privileges:true \
      -v "$src:/src:ro" -v "$root/.cache:/cache" \
      -v "$src/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" \
      -w /src/apps/tool-broker/internal/broker -e DISPLAY=:91 \
      -e "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=$dsn" -e BUTLER_BROWSER_E2E=1 \
      -e BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs \
      -e BUTLER_BROWSER_E2E_BINARY=/cache/browser-runtime.test -e BUTLER_SINGLE_TAB_E2E=1 \
      -e BUTLER_SINGLE_TAB_HOST_BINARY=/cache/guarded-native-host \
      -e BUTLER_SINGLE_TAB_FIXTURE_SCRIPT=/src/apps/browser-bridge/scripts/guarded_chromium_fixture.mjs \
      -e BUTLER_SINGLE_TAB_EXTENSION=/src/web/extensions/chromium-butler \
      --entrypoint sh butler-lab-tool-browser -c 'Xvfb :91 -screen 0 1280x900x24 -nolisten tcp >/tmp/xvfb.log 2>&1 & exec /cache/browser-broker.test -test.v -test.timeout=180s -test.run="^TestResourceBrokerActualChromiumPageCASIntegration$|^TestSingleTabResourceActualChromiumNativeHostIntegration$"'
    ;;
  probes)
    webfetch="$src/apps/tool-webfetch/internal/runtime/audit_probe_test.go"
    http="$src/apps/tool-http/internal/runtime/audit_probe_test.go"
    [[ ! -e "$webfetch" && ! -e "$http" ]] || exit 2
    trap 'rm -f -- "$webfetch" "$http"' EXIT
    cp "$src/scripts/dev/web-tools-audit-fixtures/webfetch_test.go.txt" "$webfetch"
    cp "$src/scripts/dev/web-tools-audit-fixtures/http_test.go.txt" "$http"
    go_run go test -json -p 1 -count=1 -run '^TestAudit' ./apps/tool-webfetch/internal/runtime ./apps/tool-http/internal/runtime
    ;;
  web)
    docker run --rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 \
      --shm-size 256m --security-opt no-new-privileges:true -v "$src:/src" \
      -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/ms-playwright:/browsers:ro \
      -w /src/web -e NPM_CONFIG_CACHE=/cache/npm -e PLAYWRIGHT_BROWSERS_PATH=/browsers \
      --entrypoint bash butler-lab-tool-browser -eu -c '
        npm ci
        npm run typecheck
        npm run lint
        npx --no-install playwright test --workers=1 tests/e2e/generated-converters.spec.ts
      '
    ;;
  *) exit 2 ;;
esac

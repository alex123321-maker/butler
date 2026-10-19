#!/usr/bin/env bash
set -euo pipefail
root=${1:?isolated server-browser agent root required}
mode=${2:?unit|browser|broker|web|web-real required}
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
mkdir -p "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/.cache/docker" "$root/reports"
export DOCKER_CONFIG="$root/.cache/docker"
exec 9>"$root/.cache/validation.lock"
flock 9
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
exec > >(tee "$root/reports/$mode-$run.log") 2>&1
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
browser_image=$(docker image inspect --format '{{.Id}}' butler-lab-tool-browser)
printf 'source=%s mode=%s go=%s browser=%s\n' "$src" "$mode" "$go_image" "$browser_image"
go_args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256
 --security-opt no-new-privileges:true -v "$src:/src:ro" -v "$root/.cache:/cache"
 -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off)
browser_args=(--rm --user "$(id -u):$(id -g)" --memory 2g --cpus 2 --pids-limit 256 --shm-size 256m
 --security-opt no-new-privileges:true -v "$src:/src:ro" -v "$root/.cache:/cache"
 -v "$src/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" -v "$src/configs:/app/configs:ro"
 -e BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs)
case "$mode" in
 unit)
  docker run "${go_args[@]}" --network none "$go_image" go test -json -race -p 1 -count=1 \
   -skip '^TestBrowserRuntimeEgressHarness$|^TestBrowserSessionE2E$|^TestServerBrowserChromium$|^TestFunctionCandidatePassesOrdinaryBrokerSchemaOnly$' \
   ./internal/browsercontrol ./internal/security/egress ./internal/config ./internal/actionauth ./apps/tool-browser/... ./apps/tool-broker/...
  docker run "${go_args[@]}" --network none "$go_image" go test -json -race -p 1 -count=1 \
   -skip '^TestServerBrowserOwnerAPIHarness$' -run '^TestServerBrowser|^TestBrowserOwnerFence|^TestAssembler|^TestBrowserStrategy' ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/prompt
  docker run "${go_args[@]}" --network none "$go_image" go vet ./internal/browsercontrol ./internal/security/egress ./internal/config ./internal/actionauth ./apps/tool-browser/... ./apps/tool-broker/... ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app
  ;;
 browser|broker)
  docker run "${go_args[@]}" --network none -e CGO_ENABLED=0 "$go_image" go test -c -o /cache/browser.test ./apps/tool-browser/internal/runtime
  if [[ "$mode" == browser ]]; then
   docker run "${browser_args[@]}" --network none -w /src/apps/tool-browser/internal/runtime -e BUTLER_BROWSER_E2E=1 \
    --entrypoint /cache/browser.test "$browser_image" -test.v -test.timeout=180s -test.run '^TestServerBrowserChromium$|^TestBrowserSessionE2E$'
  else
   docker run "${go_args[@]}" --network none -e CGO_ENABLED=0 "$go_image" go test -c -o /cache/broker.test ./apps/tool-broker/internal/broker
   pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
   pg="butler-server-browser-pg-$run"
   created=false
   cleanup() { if $created; then docker rm -f "$pg" >/dev/null; fi; }
   trap cleanup EXIT
   mkdir -p "$root/.cache/pg-$run"
   docker run -d --name "$pg" --network none --memory 512m --cpus 1 --pids-limit 128 \
    -v "$root/.cache/pg-$run:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=browser_test "$pg_image" -c max_connections=40 >/dev/null
   created=true
   for i in $(seq 1 30); do docker exec "$pg" pg_isready -U postgres -d browser_test >/dev/null 2>&1 && break; sleep 1; done
   docker exec "$pg" pg_isready -U postgres -d browser_test
   docker run "${browser_args[@]}" --network "container:$pg" -w /src/apps/tool-broker/internal/broker \
    -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/browser_test?sslmode=disable' \
    -e BUTLER_SERVER_BROWSER_E2E=1 -e BUTLER_BROWSER_E2E_BINARY=/cache/browser.test \
    --entrypoint /cache/broker.test "$browser_image" -test.v -test.timeout=180s -test.run '^TestServerBrowserBroker'
  fi
  ;;
 web)
  docker run --rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --shm-size 256m \
   --security-opt no-new-privileges:true -v "$src:/src" -v "$root/.cache:/cache" \
   -v /srv/storage/butler/.cache/ms-playwright:/browsers:ro -w /src/web \
   -e NPM_CONFIG_CACHE=/cache/npm -e PLAYWRIGHT_BROWSERS_PATH=/browsers --entrypoint bash "$browser_image" -eu -c '
    npm ci
    npm run typecheck
    npm run lint
    npx --no-install playwright test --workers=1 tests/e2e/server-browser.spec.ts
   '
  ;;
 web-real)
  docker run "${go_args[@]}" --network none -e CGO_ENABLED=0 "$go_image" go test -c -o /cache/browser.test ./apps/tool-browser/internal/runtime
  docker run "${go_args[@]}" --network none -e CGO_ENABLED=0 "$go_image" go test -c -o /cache/broker.test ./apps/tool-broker/internal/broker
  docker run "${go_args[@]}" --network none -e CGO_ENABLED=0 "$go_image" go test -c -o /cache/owner-api.test ./apps/orchestrator/internal/api
  docker run --rm --user "$(id -u):$(id -g)" --memory 2g --cpus 2 --pids-limit 256 -v "$src:/src" -v "$root/.cache:/cache" \
   -w /src/web -e NPM_CONFIG_CACHE=/cache/npm --entrypoint bash "$browser_image" -eu -c 'npm ci; npm run typecheck; npm run lint'
  pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
  pg="butler-server-browser-ui-pg-$run"
  created=false
  cleanup() { if $created; then docker rm -f "$pg" >/dev/null; fi; }
  trap cleanup EXIT
  mkdir -p "$root/.cache/pg-$run"
  docker run -d --name "$pg" --network none --memory 512m --cpus 1 --pids-limit 128 \
   -v "$root/.cache/pg-$run:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=browser_test "$pg_image" -c max_connections=40 >/dev/null
  created=true
  for i in $(seq 1 30); do docker exec "$pg" pg_isready -U postgres -d browser_test >/dev/null 2>&1 && break; sleep 1; done
  docker exec "$pg" pg_isready -U postgres -d browser_test
  docker run "${browser_args[@]}" --network "container:$pg" -v "$src/web:/src/web" \
   -v /srv/storage/butler/.cache/ms-playwright:/browsers:ro -w /src/web \
   -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/browser_test?sslmode=disable' \
   -e BUTLER_SERVER_BROWSER_E2E=1 -e BUTLER_BROWSER_E2E_BINARY=/cache/browser.test \
   -e BUTLER_SERVER_BROWSER_API_BINARY=/cache/owner-api.test -e BUTLER_SERVER_BROWSER_UI_API_ADDR=127.0.0.1:4181 \
   -e BUTLER_SERVER_BROWSER_UI_DONE="/cache/ui-real-$run.done" -e BUTLER_SERVER_BROWSER_REAL_API=http://127.0.0.1:4181 \
   -e PLAYWRIGHT_BROWSERS_PATH=/browsers -e NPM_CONFIG_CACHE=/cache/npm --entrypoint bash "$browser_image" -eu -c '
    (cd /src/apps/tool-broker/internal/broker; PLAYWRIGHT_BROWSERS_PATH=/ms-playwright /cache/broker.test -test.v -test.timeout=150s -test.run="^TestServerBrowserBrokerIntegration$") > /cache/ui-broker.log 2>&1 &
    broker_pid=$!
    trap '\''kill "$broker_pid" 2>/dev/null || true'\'' EXIT
    ready=false
    for i in $(seq 1 80); do if (echo > /dev/tcp/127.0.0.1/4181) 2>/dev/null; then ready=true; break; fi; sleep 0.25; done
    if ! $ready; then cat /cache/ui-broker.log; exit 1; fi
    npx --no-install playwright test --workers=1 tests/e2e/server-browser-real.spec.ts
    touch "$BUTLER_SERVER_BROWSER_UI_DONE"
    wait "$broker_pid"
    cat /cache/ui-broker.log
   '
  ;;
 *) exit 2 ;;
esac

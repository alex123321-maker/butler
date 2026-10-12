#!/usr/bin/env bash
# Actual Chromium -> Go owner HTTP -> disposable PG, no provider or main services.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/web/tests/e2e/owner-real-api.spec.ts" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/owner-browser-api-$run_id"
name="butler-owner-browser-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" /srv/storage/butler/.cache/go-tmp /srv/storage/butler/.cache/npm /srv/storage/butler/.cache/ms-playwright
created_network=false
cleanup() {
  docker rm -f "$name-api" "$name-pg" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export DOCKER_CONFIG="$root/.cache/docker"
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$name-pg" --network "$network" --network-alias owner-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=owner-fixture --env POSTGRES_DB=butler_owner_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
for attempt in $(seq 1 60); do
  if docker exec "$name-pg" pg_isready -U fixture -d butler_owner_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Actual owner browser API proof: %s\n' "$proof"
docker run -d --name "$name-api" --user "$(id -u):$(id -g)" --network "$network" --network-alias owner-ui-api \
  --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$proof:/proof" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/cache/go-tmp \
  --env BUTLER_OWNER_UI_FIXTURE_SERVER=1 \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:owner-fixture@owner-postgres:5432/butler_owner_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c 'go test -json -race -p 1 -count=1 -timeout=15m -run "^TestBrowserOwnerStateFixtureServer$" ./apps/orchestrator/internal/api > /proof/api.jsonl' >/dev/null
for attempt in $(seq 1 120); do
  if docker run --rm --network "$network" --entrypoint node butler-lab-tool-browser -e 'fetch("http://owner-ui-api:8080/fixture/state").then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))' >/dev/null 2>&1; then break; fi
  [[ "$(docker inspect --format '{{.State.Running}}' "$name-api")" == true ]] || { tail -n 30 "$proof/api.jsonl"; exit 1; }
  [[ "$attempt" != 120 ]] || exit 1
  sleep 1
done
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 2g --cpus 2 --pids-limit 256 --shm-size 256m \
  --security-opt no-new-privileges:true --volume "$source_dir:/src" --volume /srv/storage/butler/.cache:/cache --workdir /src/web \
  --env NPM_CONFIG_CACHE=/cache/npm --env PLAYWRIGHT_BROWSERS_PATH=/cache/ms-playwright \
  --env BUTLER_API_BASE_URL=http://owner-ui-api:8080 --env BUTLER_OWNER_REAL_API=1 \
  --entrypoint bash butler-lab-tool-browser -eu -c 'npm ci --offline; npx --no-install playwright test --workers=1 tests/e2e/owner-real-api.spec.ts' > "$proof/browser.log" 2>&1
[[ "$(docker wait "$name-api")" == 0 ]] || { tail -n 35 "$proof/api.jsonl"; exit 1; }
! grep -q '"Action":"skip"' "$proof/api.jsonl"
tail -n 12 "$proof/browser.log"
sha256sum "$proof/api.jsonl" "$proof/browser.log"
printf '{"status":"actual_owner_browser_api_passed","model_calls":0,"real_human_messages":0}\n'

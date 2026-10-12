#!/usr/bin/env bash
# Isolated synthetic Chrome/extension/native-host + actual Broker/PG acceptance.
set -euo pipefail
root=${1:?isolated root required}
[[ "$root" == /srv/storage/butler/agents/resource-runtime-bridge || "$root" == /srv/storage/butler/agents/resource-full-acceptance-20261012 ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/single-tab-$run_id"
name="butler-single-tab-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$proof/chrome-tmp" "$proof/go-tmp" "$root/.cache"
export DOCKER_CONFIG="$root/.cache/docker"
created_pg=false; created_network=false
cleanup(){
 $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
 $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal "$network" >/dev/null;created_network=true
docker run -d --name "$name" --network "$network" --network-alias tab-postgres --memory 512m --pids-limit 64 --cpus 1 \
 --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=tab-fixture --env POSTGRES_DB=butler_tab_test \
 --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=32 >/dev/null;created_pg=true
for i in $(seq 1 60);do docker exec "$name" pg_isready -U fixture -d butler_tab_test >/dev/null 2>&1 && break;sleep 1;done
dsn='postgres://fixture:tab-fixture@tab-postgres:5432/butler_tab_test?sslmode=disable'
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --cpus 2 \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$proof/go-tmp:/private-tmp" --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --env TMPDIR=/private-tmp \
 --env "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=$dsn" --env "BUTLER_TEST_POSTGRES_URL=$dsn" --env "BUTLER_TEST_MEMORY_POSTGRES_URL=$dsn" \
 golang:1.27.2-bookworm go test -json -race -p=1 -count=1 -timeout=120s \
 -run '^TestSingleTabResource|^TestRuntimePrivate|^TestAuthorityHTTP|^TestDispatcher|^TestGuarded' \
 ./internal/resourceexec ./internal/singletabcontrol ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/singletab \
 ./apps/browser-bridge/internal/bridge ./apps/browser-bridge/internal/protocol ./apps/browser-bridge/internal/app ./apps/browser-bridge/cmd/guarded > "$proof/pg-race.jsonl"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --cpus 2 \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$proof/go-tmp:/private-tmp" --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
 golang:1.27.2-bookworm go test -json -race -p=1 -count=1 -timeout=120s ./apps/browser-bridge/... > "$proof/bridge-race.jsonl"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --cpus 2 \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$root/.cache:/proof-cache" --workdir /src \
 --env CGO_ENABLED=0 --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 golang:1.27.2-bookworm bash -c 'go build -o /proof-cache/guarded-native-host ./apps/browser-bridge/cmd/guarded && go test -c -o /proof-cache/single-tab-broker.test ./apps/tool-broker/internal/broker'
# Only this uniquely named temporary fixture directory is made writable by the
# existing browser image UID. It contains no user account/profile or credential.
docker run --rm --network none --user 0 --volume "$proof/chrome-tmp:/owned-tmp" --entrypoint sh butler-lab-tool-browser -c 'chown 10001:10001 /owned-tmp; chmod 700 /owned-tmp'
docker run --rm --user 10001:10001 --network "$network" --memory 1536m --pids-limit 256 --cpus 2 \
 --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume "$root/.cache:/proof-cache:ro" \
 --volume "$proof/chrome-tmp:/tmp" --workdir /src/apps/tool-broker/internal/broker \
 --env DISPLAY=:91 --env BUTLER_SINGLE_TAB_E2E=1 --env BUTLER_SINGLE_TAB_HOST_BINARY=/proof-cache/guarded-native-host \
 --env BUTLER_SINGLE_TAB_FIXTURE_SCRIPT=/src/apps/browser-bridge/scripts/guarded_chromium_fixture.mjs \
 --env BUTLER_SINGLE_TAB_EXTENSION=/src/web/extensions/chromium-butler --env "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=$dsn" \
 --entrypoint sh butler-lab-tool-browser -c 'Xvfb :91 -screen 0 1280x900x24 -nolisten tcp >/tmp/xvfb.log 2>&1 & exec /proof-cache/single-tab-broker.test -test.v -test.timeout=150s -test.run="^TestSingleTabResourceActualChromiumNativeHostIntegration$"' > "$proof/chromium.log"
sha256sum "$proof/pg-race.jsonl" "$proof/bridge-race.jsonl" "$proof/chromium.log" > "$proof/events.sha256"
printf 'Single-tab actual synthetic proof passed: %s\n' "$proof"

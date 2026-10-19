#!/usr/bin/env bash
# Dedicated network-none PG and test processes; never uses shared Compose.
set -euo pipefail
root=${1:?fresh owned stage1 agent root required}
mode=${2:?unit|broker required}
[[ "$root" =~ ^/srv/storage/butler/agents/web-tools-stage1-[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
mkdir -p "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/.cache/docker" "$root/reports"
export DOCKER_CONFIG="$root/.cache/docker"
exec 9>"$root/.cache/validation.lock"
flock 9
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
report="$root/reports/$mode-$run.log"
exec > >(tee "$report") 2>&1
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
printf 'source=%s mode=%s go_image=%s report=%s\n' "$src" "$mode" "$go_image" "$report"
go_args=(--rm --user "$(id -u):$(id -g)" --memory 2g --cpus 2 --pids-limit 256
  --security-opt no-new-privileges:true -v "$src:/src:ro" -v "$root/.cache:/cache"
  -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
  -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp
  -e GOPROXY=off -e GOSUMDB=off)
case "$mode" in
  unit)
    docker run "${go_args[@]}" --network none "$go_image" go test -json -race -p 1 -count=1 \
      -skip '^TestExperienceHTTPRuntimeHarness$|^TestWebFetchRuntimeHarness$' \
      ./apps/tool-http/... ./apps/tool-webfetch/...
    docker run "${go_args[@]}" --network none "$go_image" go vet ./apps/tool-http/... ./apps/tool-webfetch/...
    ;;
  broker)
    docker run "${go_args[@]}" --network none "$go_image" go test -race -c -o /cache/http.test ./apps/tool-http/internal/runtime
    docker run "${go_args[@]}" --network none "$go_image" go test -race -c -o /cache/webfetch.test ./apps/tool-webfetch/internal/runtime
    pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
    pg="butler-web-stage1-pg-$run"
    created=false
    cleanup() { if $created; then docker rm -f "$pg" >/dev/null; fi; }
    trap cleanup EXIT
    mkdir -p "$root/.cache/pg-$run"
    printf 'pg_image=%s pg=%s\n' "$pg_image" "$pg"
    docker run -d --name "$pg" --network none --memory 512m --cpus 1 --pids-limit 128 \
      -v "$root/.cache/pg-$run:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust \
      -e POSTGRES_DB=web_stage1_test "$pg_image" -c shared_buffers=32MB -c max_connections=40 >/dev/null
    created=true
    for i in $(seq 1 30); do
      docker exec "$pg" pg_isready -U postgres -d web_stage1_test >/dev/null 2>&1 && break
      sleep 1
    done
    docker exec "$pg" pg_isready -U postgres -d web_stage1_test
    docker run "${go_args[@]}" --network "container:$pg" \
      -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/web_stage1_test?sslmode=disable' \
      -e BUTLER_WEB_TOOLS_BROKER_E2E=1 -e BUTLER_WEB_TOOLS_HTTP_BINARY=/cache/http.test \
      -e BUTLER_WEB_TOOLS_WEBFETCH_BINARY=/cache/webfetch.test "$go_image" \
      go test -json -race -p 1 -count=1 -timeout=180s \
      -run '^TestWebToolsBroker|^TestReadFailure|^TestCompletedRuntimeResult|^TestNetworkCapDenies|^TestBrokerPostgresLostExternal' \
      ./apps/tool-broker/internal/broker
    docker run "${go_args[@]}" --network none "$go_image" go vet ./apps/tool-broker/internal/broker
    ;;
  *) exit 2 ;;
esac

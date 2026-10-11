#!/usr/bin/env bash
set -euo pipefail

# Disposable isolated image/container; never touches the shared Compose stack.
converter_root=${1:?isolated absolute agent root required}
case "$converter_root" in
  /srv/storage/butler/agents/browser-sessions) ;;
  *) echo 'isolated browser-sessions root required' >&2; exit 1 ;;
esac
converter_source=$(readlink -f "$converter_root/current")
converter_image=butler-agent-browser-sessions-converter
converter_container="butler-agent-converter-sandbox-$$"
converter_sandbox="$converter_root/.cache/converter-sandbox-$$"
mkdir -m 700 "$converter_sandbox"
trap 'docker rm -f "$converter_container" >/dev/null 2>&1 || true' EXIT
export DOCKER_CONFIG=/srv/storage/butler/.cache/docker
docker buildx build --builder "${BUTLER_CONVERTER_BUILD_BUILDER:-butler-lab-storage-624adced0a63}" --load \
  --file "$converter_source/docker/tool-converter/Dockerfile" --tag "$converter_image" "$converter_source"
converter_digest=$(docker run --rm --network none --read-only --cap-drop ALL \
  --entrypoint sha256sum "$converter_image" /usr/local/bin/tool-converter | cut -d' ' -f1)
converter_profile='{"dependency":{"id":"butler-converter-ir","version":"1.0.0","sha256":"'"$converter_digest"'"},"checker":"butler.delimited-json.checks.v1"}'
printf '%s' 'isolated-converter-sandbox-token-000000000000' > "$converter_sandbox/runtime.token"
chmod 444 "$converter_sandbox/runtime.token"
docker run --rm --user 1000:1000 --network butler-lab_backend \
  --volume "$converter_source:/src:ro" --volume "$converter_root/.cache:/cache" --workdir /src \
  --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  golang:1.27.2-alpine go test -c -o "/cache/converter-runtime.test" ./apps/tool-converter/internal/runtime
docker run --detach --name "$converter_container" --network none --read-only \
  --cap-drop ALL --security-opt no-new-privileges:true --pids-limit 64 --memory 128m \
  --tmpfs /tmp:rw,noexec,nosuid,nodev,size=4m,uid=10001,gid=10001,mode=0700 \
  --mount "type=bind,src=$converter_sandbox/runtime.token,dst=/run/secrets/converter-runtime,readonly" \
  --mount "type=bind,src=$converter_root/.cache/converter-runtime.test,dst=/tests/converter-runtime.test,readonly" \
  --env "BUTLER_CONVERTER_PROFILE_JSON=$converter_profile" \
  --env BUTLER_CONVERTER_RUNTIME_TOKEN_FILE=/run/secrets/converter-runtime "$converter_image" >/dev/null
for converter_attempt in $(seq 1 30); do
  if docker exec "$converter_container" wget -qO- http://127.0.0.1:8080/health >/dev/null; then break; fi
  if [ "$converter_attempt" = 30 ]; then echo 'sandbox runtime health unavailable' >&2; exit 1; fi
  sleep 0.1
done
docker exec "$converter_container" sh -ec '
  test "$(id -u)" = 10001
  test ! -e /var/run/docker.sock
  test ! -e /srv/storage
  test ! -e /src
  test ! -e /root/.ssh
  test -z "${BUTLER_POSTGRES_URL:-}${BUTLER_REDIS_URL:-}${OPENAI_API_KEY:-}${BUTLER_OWNER_CONTROL_TOKEN:-}${BUTLER_ACTION_AUTHORITY_TOKEN_FILE:-}"
  grep -q "^CapEff:[[:space:]]*0000000000000000$" /proc/self/status
  grep -q "^NoNewPrivs:[[:space:]]*1$" /proc/self/status
  if (echo denied > /etc/converter-write) 2>/dev/null; then exit 1; fi
  if (echo denied > /run/secrets/converter-runtime) 2>/dev/null; then exit 1; fi
  if wget -T 1 -qO- http://192.0.2.1/ >/dev/null 2>&1; then exit 1; fi
'
docker exec --env BUTLER_CONVERTER_CONTAINER_E2E=1 "$converter_container" \
  /tests/converter-runtime.test -test.v -test.timeout=30s -test.run '^TestContainerConverterSmoke$'
test "$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}|{{.HostConfig.NetworkMode}}|{{len .Mounts}}|{{.Config.User}}' "$converter_container")" = 'true|none|2|butler'
test -z "$(docker exec "$converter_container" ls -A /tmp)"
printf 'PASS: actual pure-IR developer/checker/converter; UID10001, no capabilities/escalation/hostFS/Docker/PG/model/owner credentials; network none; bounded noexec workspace removed\n'

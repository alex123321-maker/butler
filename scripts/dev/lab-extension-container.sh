#!/usr/bin/env bash
set -euo pipefail

# Explicit disposable image/container only. Never starts the shared Compose app.
extension_root=${1:?isolated absolute agent root required}
case "$extension_root" in
  /srv/storage/butler/agents/browser-sessions) ;;
  *) echo 'isolated browser-sessions root required' >&2; exit 1 ;;
esac
extension_source=$(readlink -f "$extension_root/current")
extension_image=butler-agent-browser-sessions-extension
extension_container="butler-agent-extension-sandbox-$$"
extension_sandbox="$extension_root/.cache/extension-sandbox-$$"
mkdir -m 700 "$extension_sandbox"
trap 'docker rm -f "$extension_container" >/dev/null 2>&1 || true' EXIT
export DOCKER_CONFIG=/srv/storage/butler/.cache/docker
docker buildx build --builder "${BUTLER_EXTENSION_BUILD_BUILDER:-butler-lab-storage-624adced0a63}" --load \
  --file "$extension_source/docker/tool-extension/Dockerfile" \
  --tag "$extension_image" "$extension_source"
extension_digest=$(docker run --rm --network none --read-only --cap-drop ALL \
  --entrypoint sha256sum "$extension_image" /usr/local/bin/tool-extension | cut -d' ' -f1)
extension_profiles='{"reference-http":{"target":"tool-extension:9090","dependency":{"id":"reviewed-http-runtime","version":"1.0.0","source":"https://github.com/alex123321-maker/butler/tree/f9bd1ec90eca896733ed1166421e2e6f973f2f4b/apps/tool-extension","sha256":"'"$extension_digest"'"}}}'
# Synthetic token only; no operator/provider/native credential is read.
printf '%s' 'isolated-extension-sandbox-token-000000000000' > "$extension_sandbox/runtime.token"
chmod 444 "$extension_sandbox/runtime.token"
docker run --detach --name "$extension_container" --network none --read-only \
  --cap-drop ALL --security-opt no-new-privileges:true --pids-limit 64 --memory 256m \
  --mount "type=bind,src=$extension_sandbox/runtime.token,dst=/run/secrets/extension-runtime,readonly" \
  --env "BUTLER_EXTENSION_PROFILES_JSON=$extension_profiles" \
  --env BUTLER_EXTENSION_RUNTIME_TOKEN_FILE=/run/secrets/extension-runtime \
  "$extension_image" >/dev/null
for extension_attempt in $(seq 1 30); do
  if docker exec "$extension_container" wget -qO- http://127.0.0.1:8080/health >/dev/null; then break; fi
  if [ "$extension_attempt" = 30 ]; then echo 'sandbox runtime health unavailable' >&2; exit 1; fi
  sleep 0.1
done
docker exec "$extension_container" sh -ec '
  test "$(id -u)" = 10001
  test ! -e /var/run/docker.sock
  test ! -e /srv/storage
  test ! -e /src
  test ! -e /root/.ssh
  test -z "${BUTLER_POSTGRES_URL:-}${BUTLER_REDIS_URL:-}${OPENAI_API_KEY:-}${BUTLER_OWNER_CONTROL_TOKEN:-}${BUTLER_ACTION_AUTHORITY_TOKEN_FILE:-}"
  grep -q "^CapEff:[[:space:]]*0000000000000000$" /proc/self/status
  grep -q "^NoNewPrivs:[[:space:]]*1$" /proc/self/status
  if (echo denied > /etc/extension-sandbox-write) 2>/dev/null; then exit 1; fi
  if (echo denied > /run/secrets/extension-runtime) 2>/dev/null; then exit 1; fi
'
test "$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}|{{.HostConfig.NetworkMode}}|{{len .Mounts}}|{{.Config.User}}' "$extension_container")" = 'true|none|1|butler'
printf 'PASS: real executor healthy, UID10001, root/token read-only, no capabilities/escalation/hostFS/Docker/PG/model/owner credentials; network none\n'

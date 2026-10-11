#!/usr/bin/env bash
# Metadata only; every generation is denied before upstream.
set -euo pipefail
root=${1:?agent root required}
[[ "$root" == /srv/storage/butler/agents/antigravity-desktop ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
export DOCKER_CONFIG="$root/.cache/docker"
umask 077
name=butler-agy-desktop-fixture
gateway=butler-agy-desktop-metadata
network=butler-agy-desktop-fixture-net
common=(--runtime runc --pull never --read-only --cap-drop ALL --security-opt no-new-privileges
 --memory 2g --pids-limit 256 --cpus 2 --tmpfs /tmp:rw,nosuid,nodev,size=256m)
docker run --rm "${common[@]}" --network none --user "$(id -u):$(id -g)" \
 --env GOCACHE=/cache/go-build --env GOMODCACHE=/modules --env GOTMPDIR=/cache/go-tmp --env CGO_ENABLED=0 \
 --mount "type=bind,source=$source_dir,target=/src,readonly" --mount "type=bind,source=$root/.cache,target=/cache" \
 --mount type=bind,source=/srv/storage/butler/.cache/go-mod,target=/modules,readonly \
 --mount "type=bind,source=$root/runtime,target=/out" --workdir /src golang:1.27.2-bookworm \
 go build -o /out/desktop-fixture ./scripts/dev/antigravity-desktop
mkdir -p "$root/private"
docker run --rm --pull never --network none --mount "type=bind,source=$root/private,target=/private" alpine:3.22 \
 sh -eu -c 'chown 10001:10001 /private; chmod 700 /private'
docker network create --internal "$network" >/dev/null
auth_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
docker create "${common[@]}" --name "$gateway" --user 10001:10001 --network "$network" --network-alias desktop-metadata \
 --mount "type=bind,source=$root/runtime/desktop-fixture,target=/fixture,readonly" \
 --mount type=bind,source=/srv/storage/butler/agents/antigravity-compare/private/control.env,target=/operator/control.env,readonly \
 --mount "type=bind,source=$auth_parent/butler-native-import.json,target=/auth/import.json,readonly" \
 --mount "type=bind,source=$auth_parent/antigravity-oauth-token,target=/auth/cli.json,readonly" \
 --mount "type=bind,source=$root/private,target=/private-desktop" \
 golang:1.27.2-bookworm /fixture metadata-gate >/dev/null
docker network connect butler-lab_backend "$gateway"
docker start "$gateway" >/dev/null
ready=false
for i in $(seq 1 25);do
 if docker logs "$gateway" | grep -q '"metadata_gateway_ready":true';then ready=true;break;fi
 [[ "$(docker inspect --format '{{.State.Running}}' "$gateway")" == true ]] || break
 sleep 1
done
$ready || { printf '{"metadata_ready":false,"real_model_calls":0}\n';exit 1; }
docker run -d "${common[@]}" --name "$name" --network "$network" --user 10001:10001 \
 --env BUTLER_DESKTOP_OFFLINE=1 --env BUTLER_DESKTOP_METADATA=1 \
 --mount "type=bind,source=$root/runtime/desktop-fixture,target=/fixture,readonly" \
 --mount "type=bind,source=$root/runtime/language_server,target=/language_server,readonly" \
 --mount "type=bind,source=$root/private,target=/private-desktop" \
 golang:1.27.2-bookworm /fixture fixture >/dev/null
nohup node "$source_dir/scripts/dev/antigravity-desktop/fixture-relay.mjs" > "$root/reports/metadata-relay.jsonl" 2> "$root/reports/metadata-relay-errors" < /dev/null &
printf '%s\n' "$!" > "$root/runtime/fixture-relay.pid"
printf '{"metadata_ready":true,"generation_forwarding_enabled":false,"real_model_calls":0}\n'

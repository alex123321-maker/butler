#!/usr/bin/env bash
# Isolated synthetic Desktop transport. No live forwarding, account or model usage.
set -euo pipefail
root=${1:?agent root required}
[[ "$root" == /srv/storage/butler/agents/antigravity-desktop ]] || exit 2
phase=${2:-tests}
[[ "$phase" == tests || "$phase" == native || "$phase" == build || "$phase" == fixture || "$phase" == report || "$phase" == stop ]] || exit 2
name=butler-agy-desktop-fixture
network=butler-agy-desktop-fixture-net
export DOCKER_CONFIG="$root/.cache/docker"
mkdir -p "$root/.cache/docker" "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/runtime" "$root/reports"
if [[ "$phase" == report ]];then
 docker exec "$name" /fixture report
 exit 0
fi
if [[ "$phase" == stop ]];then
 if [[ -f "$root/runtime/fixture-relay.pid" ]];then
  relay_pid=$(cat "$root/runtime/fixture-relay.pid")
  if [[ "$relay_pid" =~ ^[0-9]+$ ]] && [[ -r "/proc/$relay_pid/cmdline" ]] && tr '\0' ' ' < "/proc/$relay_pid/cmdline" | grep -q '/antigravity-desktop/fixture-relay.mjs';then kill "$relay_pid";fi
 fi
 stop_proof="$root/reports/desktop-stop-$(date -u +%Y%m%dT%H%M%SZ)-$$"
 if docker inspect "$name" >/dev/null 2>&1;then
  docker exec "$name" /fixture report > "$stop_proof.json" || true
  docker logs "$name" > "$stop_proof-fixture.jsonl"
  docker rm -f "$name" >/dev/null
 fi
 if docker inspect butler-agy-desktop-metadata >/dev/null 2>&1;then
  docker logs butler-agy-desktop-metadata > "$stop_proof-metadata.jsonl"
  docker rm -f butler-agy-desktop-metadata >/dev/null
 fi
 docker network rm "$network" >/dev/null
 printf '{"owned_diagnostic_containers_stopped":true,"safe_reports_preserved":true}\n'
 exit 0
fi
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
common=(--runtime runc --pull never --read-only --cap-drop ALL --security-opt no-new-privileges
 --memory 2g --pids-limit 256 --cpus 2 --tmpfs /tmp:rw,nosuid,nodev,size=256m)
build=(--user "$(id -u):$(id -g)" --env GOCACHE=/cache/go-build --env GOMODCACHE=/modules --env GOTMPDIR=/cache/go-tmp
 --mount "type=bind,source=$source_dir,target=/src,readonly"
 --mount "type=bind,source=$root/.cache,target=/cache"
 --mount type=bind,source=/srv/storage/butler/.cache/go-mod,target=/modules,readonly --workdir /src)
proof="$root/reports/desktop-$(date -u +%Y%m%dT%H%M%SZ)-$$-$phase"
docker run --rm "${common[@]}" "${build[@]}" --network none golang:1.27.2-bookworm go vet ./scripts/dev/antigravity-desktop
if [[ "$phase" == tests ]];then
 docker run --rm "${common[@]}" "${build[@]}" --network none golang:1.27.2-bookworm go test -json -race -p 1 -count=1 ./scripts/dev/antigravity-desktop > "$proof.jsonl"
 sha256sum "$proof.jsonl"
 exit 0
fi
[[ "$(sha256sum "$root/runtime/language_server"|cut -d ' ' -f1)" == bbb745ed391e82bb18ed85ceef36bb646a2a0cf801903bc1ef0b8e6df8ba1bc8 ]] || exit 2
docker run --rm "${common[@]}" "${build[@]}" --network none --env CGO_ENABLED=0 --mount "type=bind,source=$root/runtime,target=/out" golang:1.27.2-bookworm go build -o /out/desktop-fixture ./scripts/dev/antigravity-desktop
[[ "$phase" != build ]] || exit 0
if [[ "$phase" == native ]];then
 docker run --rm "${common[@]}" --network none --user 10001:10001 --mount "type=bind,source=$root/runtime/desktop-fixture,target=/fixture,readonly" golang:1.27.2-bookworm /fixture native-fixture > "$proof.json"
 sha256sum "$proof.json"
 cat "$proof.json"
 exit 0
fi
docker network create --internal "$network" >/dev/null
docker run -d "${common[@]}" --name "$name" --network "$network" --user 10001:10001 \
 --env BUTLER_DESKTOP_OFFLINE=1 \
 --mount "type=bind,source=$root/runtime/desktop-fixture,target=/fixture,readonly" \
 --mount "type=bind,source=$root/runtime/language_server,target=/language_server,readonly" \
 golang:1.27.2-bookworm /fixture fixture > "$proof-container.id"
nohup node "$source_dir/scripts/dev/antigravity-desktop/fixture-relay.mjs" > "$proof-relay.jsonl" 2> "$proof-relay-errors" < /dev/null &
printf '%s\n' "$!" > "$root/runtime/fixture-relay.pid"
printf '{"status":"synthetic_desktop_started","real_model_calls":0,"remote_ui_port":35129}\n'

#!/usr/bin/env bash
# Captures synthetic CLI/Go wire contracts only. No operator profile or inference.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
mode=${2:-default}
[[ "$mode" == default || "$mode" == aicode || "$mode" == tests ]] || exit 2
if [[ "$mode" != tests ]]; then
  cli=/srv/storage/butler/agents/antigravity-provider/runtime/agy
  [[ "$(sha256sum "$cli" | cut -d ' ' -f 1)" == d97edcf90eda1799cd6cedc92616ef402f23c9752a723e8b8ae3e8f062ce458b ]] || {
    printf 'Installed CLI changed; review its version and fixture before capture.\n' >&2
    exit 2
  }
fi
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
export DOCKER_CONFIG="$root/.cache/docker"
mkdir -p "$root/.cache/docker" "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/runtime" "$root/reports"
proof="$root/reports/wire-$(date -u +%Y%m%dT%H%M%SZ)-$$-$mode"
common=(--rm --runtime runc --pull never --user "$(id -u):$(id -g)" --network none
  --read-only --cap-drop ALL --security-opt no-new-privileges --memory 2g
  --pids-limit 128 --cpus 2 --tmpfs /tmp:rw,nosuid,nodev,size=256m)
build=(--env GOCACHE=/cache/go-build --env GOMODCACHE=/modules --env GOTMPDIR=/cache/go-tmp
  --mount "type=bind,source=$source_dir,target=/src,readonly"
  --mount "type=bind,source=$root/.cache,target=/cache"
  --mount "type=bind,source=/srv/storage/butler/.cache/go-mod,target=/modules,readonly" --workdir /src)
if [[ "$mode" == tests ]]; then
  docker run "${common[@]}" "${build[@]}" golang:1.27.2-bookworm \
    go test -json -race -p 1 -count=1 ./scripts/dev/antigravity-wire ./internal/transport/antigravity \
    ./internal/providerfactory ./internal/providerauth ./internal/config > "$proof.jsonl"
  docker run "${common[@]}" "${build[@]}" golang:1.27.2-bookworm \
    go vet ./scripts/dev/antigravity-wire ./internal/transport/antigravity \
    ./internal/providerfactory ./internal/providerauth ./internal/config
  sha256sum "$proof.jsonl"
  exit 0
fi
docker run "${common[@]}" "${build[@]}" --mount "type=bind,source=$root/runtime,target=/out" \
  golang:1.27.2-bookworm go build -o /out/antigravity-wire ./scripts/dev/antigravity-wire
# The sole non-source input is the installed executable; no profile/env/proxy mount.
docker run "${common[@]}" --env BUTLER_ANTIGRAVITY_OFFLINE=1 --env "BUTLER_WIRE_MODE=$mode" \
  --mount "type=bind,source=$root/runtime/antigravity-wire,target=/capture,readonly" \
  --mount type=bind,source=/srv/storage/butler/agents/antigravity-provider/runtime/agy,target=/agy,readonly \
  golang:1.27.2-bookworm /capture /agy > "$proof.json"
sha256sum "$proof.json"
printf 'Synthetic wire capture: %s.json\n' "$proof"

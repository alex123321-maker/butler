#!/usr/bin/env bash
set -euo pipefail

task_root=/srv/storage/butler/agents/antigravity-provider
source_root=$(cd "$(dirname "$0")/../.." && pwd)
docker run --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$source_root,target=/src,readonly" \
  --mount "type=bind,source=$task_root/.cache,target=/cache" \
  --mount "type=bind,source=$task_root/runtime,target=/out" \
  --workdir /src --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod \
  --env CGO_ENABLED=0 golang:1.24-alpine \
  go build -o /out/antigravity-smoke ./scripts/dev/antigravity-smoke.go
exec docker run --rm --env SSH_CONNECTION \
  --mount "type=bind,source=$task_root/runtime/agy,target=/opt/antigravity/agy,readonly" \
  --mount "type=bind,source=$task_root/runtime/antigravity-smoke,target=/usr/local/bin/antigravity-smoke,readonly" \
  --mount "type=bind,source=$task_root/profile,target=/home/butler" \
  --entrypoint antigravity-smoke butler-lab-tool-browser

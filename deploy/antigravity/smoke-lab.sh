#!/usr/bin/env bash
set -euo pipefail
task_root=/srv/storage/butler/agents/antigravity-provider
source_root=$(cd "$(dirname "$0")/../.." && pwd)
control_env=${1:?A private operator environment file is required}
[[ -f "$control_env" && "$(stat -c %a "$control_env")" == 600 ]] || {
  echo 'Operator environment must be a private mode-0600 file.' >&2; exit 1;
}
mkdir -p "$task_root/.cache/go-tmp" "$task_root/runtime"
docker run --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$source_root,target=/src,readonly" \
  --mount "type=bind,source=$task_root/.cache,target=/cache" \
  --mount "type=bind,source=$task_root/runtime,target=/out" \
  --workdir /src --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env CGO_ENABLED=0 golang:1.27.2-alpine \
  go build -o /out/provider-auth ./cmd/provider-auth
exec docker run --rm --network butler-lab_backend --env-file "$control_env" \
  --mount "type=bind,source=$task_root/runtime/provider-auth,target=/usr/local/bin/provider-auth,readonly" \
  --entrypoint provider-auth butler-lab-tool-browser -action smoke

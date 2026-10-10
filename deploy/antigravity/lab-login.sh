#!/usr/bin/env bash
set -euo pipefail

# Interactive only: authorization codes are entered directly in this terminal,
# never in Butler chat, config values, or a model-visible tool call.
task_root=${1:-/srv/storage/butler/agents/antigravity-provider}
case "$task_root" in
  /srv/storage/butler/agents/antigravity-provider) ;;
  *) echo 'Unsupported lab Antigravity profile path.' >&2; exit 1 ;;
esac
test -x "$task_root/runtime/agy"
exec docker run --rm -it --env SSH_CONNECTION --workdir /work \
  --mount "type=bind,source=$task_root/runtime/agy,target=/usr/local/bin/agy,readonly" \
  --mount "type=bind,source=$task_root/profile,target=/home/butler" \
  --mount "type=bind,source=$task_root/workspace,target=/work,readonly" \
  --entrypoint agy butler-lab-tool-browser --agent butler-model

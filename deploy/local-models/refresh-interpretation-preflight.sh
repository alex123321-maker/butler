#!/usr/bin/env bash
# No predictions: exact Go prompt export and offline pinned tokenizer only.
set -euo pipefail
root="${1:?isolated interpretation root required}"
storage="${2:-/srv/storage/butler/agents/local-models}"
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ && "$root" != *..* ]] || exit 1
[[ "$storage" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ && "$storage" != *..* ]] || exit 1
src=$(realpath "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 1
[[ "$(docker inspect butler-decision-qwen-v2 --format '{{range .Mounts}}{{if eq .Destination "/models"}}{{.Source}}{{end}}{{end}}')" == "$root/models" ]] || exit 1
history="$root/models/preflight-history/initial-array"
if [[ ! -e "$history" ]]; then
 mkdir -p "$history"
 cp "$root/models/preflight-prompts.json" "$root/models/token-counts.json" "$root/models/qwen-pins.json" "$history/"
fi
docker run --rm --network none --user "$(id -u):$(id -g)" \
 --volume "$src:/src:ro" --volume "$storage/.cache:/cache" --volume "$root/models:/out" --workdir /src \
 --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --env BUTLER_TEST_QWEN_PREFLIGHT_PROMPTS=/out/preflight-prompts.json \
 --entrypoint go golang:1.27.2-bookworm test -count=1 -run '^TestWriteInterpretationQwenPreflight$' ./internal/decisions
docker run --rm --network decision-qwen-v2 --user "$(id -u):$(id -g)" \
 --volume "$storage/data:/data:ro" --volume "$root/models:/models" --volume "$src:/src:ro" \
 --env BUTLER_QWEN_PREFLIGHT_KIND=interpretation \
 --entrypoint /data/venv/bin/python "$(docker inspect butler-lab-local-models-1 --format '{{.Image}}')" \
 /src/deploy/local-models/prepare-qwen-decision-evaluation.py

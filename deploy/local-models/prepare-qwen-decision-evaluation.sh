#!/usr/bin/env bash
# Research-only pinned public artifacts; no deployed daemon, model store or port.
set -euo pipefail
root="${1:?isolated snapshot root required}"
storage="${2:-/srv/storage/butler/agents/local-models}"
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ && "$root" != *..* ]] || exit 1
[[ "$storage" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ && "$storage" != *..* ]] || exit 1
src=$(realpath "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 1
if docker inspect butler-decision-qwen-v2 >/dev/null 2>&1; then
 printf '%s\n' 'Research container already exists; preserve it and reuse its pinned artifacts, or remove that owned container before a new preparation.' >&2
 exit 1
fi
modelroot="$root/models"
mkdir -p "$modelroot/blobs"
weight=00fe7986ff5f6b463e62455821146049db6f9313603938a70800d1fb69ef11a4
file="$modelroot/blobs/sha256-$weight"
if [[ ! -f "$file" ]]; then
 curl -fsSL --retry 2 'https://huggingface.co/unsloth/Qwen3.5-4B-GGUF/resolve/e87f176479d0855a907a41277aca2f8ee7a09523/Qwen3.5-4B-Q4_K_M.gguf' -o "$file.partial"
 [[ "$(sha256sum "$file.partial" | cut -d' ' -f1)" == "$weight" ]] || exit 1
 mv "$file.partial" "$file"
fi
[[ "$(sha256sum "$file" | cut -d' ' -f1)" == "$weight" ]] || exit 1
curl -fsSL 'https://huggingface.co/Qwen/Qwen3.5-4B/resolve/851bf6e806efd8d0a36b00ddf55e13ccb7b8cd0a/tokenizer.json' -o "$modelroot/tokenizer.json"
image=sha256:b86366bb528bbf7f1424435d165028497a5b69bf6ddb4fa5a87102e2b79f44fb
docker image inspect "$image" >/dev/null
[[ "$(docker run --rm --network none --entrypoint sha256sum "$image" /usr/bin/ollama | cut -d' ' -f1)" == 8ff456e3f8c08b7fb211857c634d4df8e1a84cacaaf5b9e7b6bbf89e0f9ae8e0 ]] || exit 1
docker network create --internal decision-qwen-v2 >/dev/null
docker run -d --name butler-decision-qwen-v2 --label butler.research=decision-taxonomy-v2 \
 --network decision-qwen-v2 --network-alias decision-qwen --gpus all \
 --memory 8g --memory-swap 8g --cpus 4 --read-only --tmpfs /tmp:rw,size=512m \
 --cap-drop ALL --security-opt no-new-privileges:true --volume "$modelroot:/models" \
 --env OLLAMA_MODELS=/models --env HOME=/tmp --env OLLAMA_HOST=0.0.0.0:11434 \
 --env OLLAMA_NO_CLOUD=1 --env OLLAMA_NOHISTORY=1 --env OLLAMA_NOPRUNE=1 \
 --env OLLAMA_MAX_LOADED_MODELS=1 --env OLLAMA_NUM_PARALLEL=1 --env OLLAMA_KEEP_ALIVE=-1 \
 --env OLLAMA_CONTEXT_LENGTH=2048 --env OLLAMA_DEBUG_LOG_REQUESTS=0 \
 --env NVIDIA_DRIVER_CAPABILITIES=compute,utility --entrypoint /usr/bin/ollama "$image" serve >/dev/null
printf 'FROM /models/blobs/sha256-%s\nPARAMETER num_ctx 2048\nPARAMETER temperature 0\n' "$weight" > "$modelroot/Modelfile"
docker exec butler-decision-qwen-v2 ollama create butler-decision-qwen35-4b:q4-v2 -f /models/Modelfile
docker run --rm --network none --user "$(id -u):$(id -g)" \
 --volume "$src:/src:ro" --volume "$storage/.cache:/cache" --volume "$modelroot:/out" --workdir /src \
 --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --env BUTLER_TEST_QWEN_PREFLIGHT_PROMPTS=/out/preflight-prompts.json \
 --entrypoint go golang:1.27.2-bookworm test -count=1 -run '^TestWriteQwenTokenPreflightPrompts$' ./internal/decisions
docker run --rm --network decision-qwen-v2 --user "$(id -u):$(id -g)" \
 --volume "$storage/data:/data:ro" --volume "$modelroot:/models" --volume "$src:/src:ro" \
 --entrypoint /data/venv/bin/python "$(docker inspect butler-lab-local-models-1 --format '{{.Image}}')" \
 /src/deploy/local-models/prepare-qwen-decision-evaluation.py
printf '%s\n' 'Prepared research-only Qwen. After experiments remove only butler-decision-qwen-v2 and decision-qwen-v2; retain public pins/reports.'

#!/usr/bin/env bash
set -euo pipefail
action=${1:-check}
root=/srv/storage/butler/agents/local-generative
image=sha256:b86366bb528bbf7f1424435d165028497a5b69bf6ddb4fa5a87102e2b79f44fb
digest=7df6b6e09427a769808717c0a93cadc4ae99ed4eb8bf5ca557c90846becea435
service=butler-local-generative-observer
network=butler-local-generative-observation
export DOCKER_CONFIG=/srv/storage/butler/.cache/docker
mkdir -p "$root/reports" "$root/model-store"
[[ "$(realpath "$root")" == "$root" && "$(realpath "$root/model-store")" == "$root/model-store" ]] || exit 2
report="$root/reports/generative-$action-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir "$report"
# The Go builder image already provides curl; this read-only helper queries only
# the fixed internal service. No host port or helper dependency is installed.
metadata() {
  docker run --rm --runtime runc --network "$network" --read-only \
    --cap-drop ALL --security-opt no-new-privileges --memory 128m --cpus .2 \
    --pids-limit 32 --entrypoint curl golang:1.27.2-bookworm \
    --noproxy '*' --fail --silent --show-error --max-time 10 "$@"
}
case "$action" in
  prepare)
    docker image inspect "$image" --format '{{.Id}}' > "$report/image.txt"
    [[ "$(cat "$report/image.txt")" == "$image" ]] || exit 2
    [[ "$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)" -ge 4194304 ]] || exit 2
    # Resolve and compare the public manifest before loading any mutable tag.
    curl --fail --silent --show-error --max-time 30 \
      https://registry.ollama.ai/v2/library/qwen3/manifests/0.6b -o "$report/manifest.json"
    [[ "$(sha256sum "$report/manifest.json" | cut -d' ' -f1)" == "$digest" ]] || exit 2
    [[ -z "$(docker ps -aq --filter name=^/butler-local-generative-download$)" ]] || exit 2
    [[ -z "$(docker ps -aq --filter name=^/$service$)" ]] || exit 2
    cleanup_download() { docker rm -f butler-local-generative-download >/dev/null 2>&1 || true; }
    trap cleanup_download EXIT
    docker run -d --name butler-local-generative-download --cpus 2 --memory 2g --memory-swap 2g \
      --volume "$root/model-store:/models" --env OLLAMA_MODELS=/models \
      "$image" serve > "$report/download-container.txt"
    for attempt in $(seq 1 30); do
      if docker exec butler-local-generative-download ollama list > /dev/null 2>&1; then break; fi
      sleep 1
    done
    timeout 480 docker exec butler-local-generative-download ollama pull qwen3:0.6b > "$report/download.log" 2>&1
    cleanup_download
    trap - EXIT
    # Public weights are readable by the unprivileged runner, mounted read-only.
    docker network inspect "$network" > /dev/null 2>&1 || docker network create --internal "$network" > "$report/network.txt"
    [[ "$(docker network inspect "$network" --format '{{.Internal}}')" == true ]] || exit 2
    docker run -d --name "$service" --runtime runc --network "$network" --network-alias butler-generative \
      --user 10001:10001 --read-only --cap-drop ALL --security-opt no-new-privileges \
      --cpus 2 --memory 2g --memory-swap 2g --pids-limit 128 \
      --tmpfs /tmp:rw,nosuid,nodev,size=64m \
      --volume "$root/model-store:/models:ro" --env HOME=/tmp --env OLLAMA_MODELS=/models \
      --env OLLAMA_NUM_PARALLEL=1 --env OLLAMA_MAX_LOADED_MODELS=1 --env OLLAMA_MAX_QUEUE=4 \
      --env OLLAMA_NO_CLOUD=true --env OLLAMA_VULKAN=false --env CUDA_VISIBLE_DEVICES=-1 \
      "$image" serve > "$report/service-container.txt"
    for attempt in $(seq 1 30); do
      if metadata http://butler-generative:11434/api/tags > "$report/tags.json"; then break; fi
      sleep 1
    done
    python3 - "$report/tags.json" "$digest" <<'PY'
import json, sys
m = json.load(open(sys.argv[1]))['models']
assert len(m) == 1 and m[0]['name'] == 'qwen3:0.6b' and m[0]['digest'] == sys.argv[2]
PY
    metadata -H 'Content-Type: application/json' \
      --data '{"model":"qwen3:0.6b"}' http://butler-generative:11434/api/show > "$report/show.json"
    ;;
  check)
    docker inspect "$service" --format '{{.Image}} {{.HostConfig.Memory}} {{.HostConfig.NanoCpus}} {{.HostConfig.ReadonlyRootfs}} {{.Config.User}}' > "$report/runtime.txt"
    [[ "$(cat "$report/runtime.txt")" == "$image 2147483648 2000000000 true 10001:10001" ]] || exit 2
    [[ "$(docker network inspect "$network" --format '{{.Internal}}')" == true ]] || exit 2
    [[ "$(docker inspect "$service" --format '{{.HostConfig.Runtime}}')" == runc ]] || exit 2
    metadata http://butler-generative:11434/api/tags > "$report/tags.json"
    metadata http://butler-generative:11434/api/ps > "$report/loaded.json"
    docker stats --no-stream "$service" --format '{{json .}}' > "$report/resources.json"
    ;;
  *) exit 2 ;;
esac
printf 'proof=%s\n' "$report"
sha256sum "$report"/*.json "$report"/*.txt 2>/dev/null || true

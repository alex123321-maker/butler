#!/usr/bin/env bash
set -euo pipefail
root=${1:-/srv/storage/butler/agents/local-models}
image=python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1
mode_filter=${2:-all}
[[ "$mode_filter" == all || "$mode_filter" == gpu || "$mode_filter" == constrained || "$mode_filter" == capabilities ]] || exit 2
[[ "$root" == /srv/storage/butler/agents/* && "$root" != *..* ]] || exit 2
mkdir -p "$root/.cache/go-tmp" "$root/runtime" "$root/private" "$root/reports"
docker run --rm --user "$(id -u):$(id -g)" --volume "$root/current:/src:ro" \
  --volume "$root/.cache:/cache" --volume "$root/runtime:/runtime" --workdir /src \
  --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --entrypoint sh golang:1.27.2-alpine -c 'set -eu; CGO_ENABLED=0 go build -o /runtime/local-models ./apps/local-models; CGO_ENABLED=0 go build -o /runtime/smoke ./scripts/dev/local-models-smoke.go; CGO_ENABLED=0 go build -o /runtime/capabilities-smoke ./scripts/dev/local-models-capabilities-smoke.go'
docker run --rm --user 0:0 --volume "$root/private:/private" --volume "$root/reports:/reports" \
  --entrypoint python "$image" -c '
import os, pathlib, secrets
p=pathlib.Path("/private/token")
if not p.exists():
    fd=os.open(p, os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600)
    with os.fdopen(fd,"w") as f: f.write(secrets.token_hex(32))
os.chown(p,10001,10001);os.chmod(p,0o600)
os.chown("/reports",10001,1000);os.chmod("/reports",0o770)
'
cleanup() {
  docker stop --time 15 butler-local-models-proof >/dev/null 2>&1 || true
  for attempt in $(seq 1 20);do
    if ! docker inspect butler-local-models-proof >/dev/null 2>&1;then return;fi
    sleep 0.2
  done
}
trap cleanup EXIT
nvidia-smi --query-gpu=name,memory.total,memory.used,memory.free,driver_version --format=csv > "$root/reports/gpu-before.csv"
uname -a > "$root/reports/os.txt"
ollama_status() {
  if docker inspect butler-local-models-ollama >/dev/null 2>&1; then
    docker exec butler-local-models-ollama ollama ps
  else
    printf 'Optional isolated Ollama compatibility probe is not installed.\n'
  fi
}
ollama_status > "$root/reports/ollama-before.txt"
for mode in gpu constrained capabilities; do
  if [[ "$mode_filter" != all && "$mode_filter" != "$mode" ]];then continue;fi
  budget=6144;expect=true;idle=3
  if [[ "$mode" == capabilities ]];then idle=30;fi
  if [[ "$mode" == constrained ]]; then budget=1024;expect=false;fi
  docker run -d --rm --init --name butler-local-models-proof --user 10001:10001 \
    --gpus all --network none --read-only --memory 8g --memory-swap 8g --pids-limit 256 --cap-drop ALL --security-opt no-new-privileges \
    --tmpfs /tmp:rw,nosuid,nodev,size=256m,uid=10001,gid=10001 \
    --volume "$root/runtime:/runtime:ro" --volume "$root/data:/data:ro" \
    --volume "$root/private:/private:ro" --volume "$root/reports:/reports" \
    --volume "$root/current/apps/local-models/model-worker.py:/app/model-worker.py:ro" \
    --volume "$root/current/deploy/local-models/models.json:/app/models.json:ro" \
    --volume "$root/current/deploy/local-models/russian-retrieval.json:/app/russian-retrieval.json:ro" \
    --env BUTLER_LOCAL_MODELS_TOKEN_FILE=/private/token --env BUTLER_LOCAL_MODELS_CATALOG=/app/models.json \
    --env BUTLER_LOCAL_MODELS_BUDGET_MIB="$budget" --env BUTLER_LOCAL_MODELS_IDLE_SECONDS="$idle" \
    --env NVIDIA_DRIVER_CAPABILITIES=compute,utility \
    --entrypoint /runtime/local-models "$image" >/dev/null
  for attempt in $(seq 1 20);do
    if docker exec butler-local-models-proof python -c 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:8080/health",timeout=2)' >/dev/null 2>&1;then break;fi
    sleep 1
  done
  if [[ "$mode" == capabilities ]];then
    docker exec butler-local-models-proof /runtime/capabilities-smoke --token-file /private/token --report /reports/verification-capabilities.json
  else
    docker exec butler-local-models-proof /runtime/smoke --token-file /private/token --report "/reports/verification-$mode.json" --expect-gpu="$expect"
  fi
  docker logs butler-local-models-proof > "$root/reports/service-$mode.log" 2>&1
  cleanup
done
nvidia-smi --query-gpu=name,memory.total,memory.used,memory.free,driver_version --format=csv > "$root/reports/gpu-after.csv"
ollama_status > "$root/reports/ollama-after.txt"
printf 'Verification artifacts: %s/reports\n' "$root"

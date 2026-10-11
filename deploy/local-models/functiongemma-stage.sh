#!/usr/bin/env bash
set -euo pipefail
# Explicit lab-only operator staging; never modifies main current or core.
root=/srv/storage/butler
own=/srv/storage/butler/agents/functiongemma
source_root=${1:?exact immutable lab source snapshot}
mode=${2:?stage or restore}
[[ "$mode" == stage || "$mode" == restore ]] || exit 2
[[ "$source_root" == /srv/storage/butler/agents/local-models/snapshots/* ]] || exit 2
exec 9>"$root/local-models/voice-stage.lock"
flock -x 9
if [[ "$mode" == restore ]]; then
 exec bash "$root/agents/voice-runtime/stage-voice.sh" restore
fi
export DOCKER_CONFIG="$root/.cache/docker"
export BUTLER_LAB_ROOT="$root"
export BUTLER_OWNER_CONTROL_TOKEN_SOURCE="$root/secrets/owner-control.token"
export BUTLER_ACTION_AUTHORITY_TOKEN_SOURCE="$root/secrets/action-authority.token"
export BUILDX_BUILDER=butler-lab-storage-624adced0a63
overlay="$own/function-evaluation.override.yml"
cat > "$overlay" <<EOF
services:
  local-models:
    image: butler-lab-local-models-function-preview
    build:
      context: $source_root
      dockerfile: docker/local-models/Dockerfile
    environment:
      BUTLER_LOCAL_MODELS_FUNCTION_EXECUTABLE: /function/cpu/bin/ollama
      BUTLER_LOCAL_MODELS_FUNCTION_MODELS: /function/models
    volumes:
      - $source_root/deploy/local-models/functiongemma-evaluation.json:/app/models.json:ro
      - $own/cpu:/function/cpu:ro
      - $own/models:/function/models:ro
EOF
args=(--project-name butler-lab --profile local-models --profile memory-parser
 --env-file "$root/lab.env" --env-file "$root/private-proxy/compose.env"
 --env-file "$root/extensions/compose.env" --env-file "$root/local-models/compose.env"
 --env-file "$root/memory-semantic/compose.env" --env-file "$root/memory-parser/compose.env"
 -f "$root/current/deploy/docker-compose.yml" -f "$root/current/deploy/docker-compose.lab.yml"
 -f "$root/current/deploy/docker-compose.antigravity-proxy.lab.yml"
 -f "$root/current/deploy/docker-compose.extensions.yml"
 -f "$root/current/deploy/docker-compose.local-models.lab.yml"
 -f "$root/current/deploy/docker-compose.memory-semantic.yml"
 -f "$root/current/deploy/docker-compose.memory-parser.yml" -f "$overlay")
docker compose "${args[@]}" build local-models
docker compose "${args[@]}" up -d --no-deps --wait local-models
docker inspect butler-lab-local-models-1 --format '{{.State.Health.Status}} {{.Config.Image}}'

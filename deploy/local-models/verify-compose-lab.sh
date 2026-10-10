#!/usr/bin/env bash
set -euo pipefail
root=${1:-/srv/storage/butler}
model_root=${2:-/srv/storage/butler/agents/local-models}
[[ "$root" == /srv/storage/butler* && "$root" != *..* ]] || exit 2
[[ "$model_root" == /srv/storage/butler/agents/* && "$model_root" != *..* ]] || exit 2
container=butler-lab-local-models-1
network=butler-lab_local-inference
[[ "$(docker inspect --format '{{.State.Health.Status}}' "$container")" == healthy ]]
[[ "$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$container")" == true ]]
[[ "$(docker inspect --format '{{.Config.User}}' "$container")" == 10001:10001 ]]
[[ "$(docker inspect --format '{{json .HostConfig.CapDrop}}' "$container")" == '["ALL"]' ]]
[[ "$(docker inspect --format '{{json .HostConfig.PortBindings}}' "$container")" == '{}' ]]
[[ "$(docker inspect --format '{{range $name,$value := .NetworkSettings.Networks}}{{$name}}{{end}}' "$container")" == "$network" ]]
[[ "$(docker network inspect --format '{{.Internal}}' "$network")" == true ]]
docker inspect --format '{{json .HostConfig.SecurityOpt}}' "$container" | grep -q 'no-new-privileges'
mkdir -p "$root/.cache/go-tmp" "$root/local-models/runtime" "$root/local-models/reports"
docker run --rm --user "$(id -u):$(id -g)" \
  --volume "$root/current:/src:ro" --volume "$root/.cache:/cache" \
  --volume "$root/local-models/runtime:/runtime" --workdir /src \
  --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --entrypoint sh golang:1.27.2-alpine -c \
  'set -eu; CGO_ENABLED=0 go build -o /runtime/capabilities-smoke ./scripts/dev/local-models-capabilities-smoke.go'
docker run --rm --user 0:0 --volume "$root/local-models/reports:/reports" \
  --entrypoint sh alpine:3.21 -c 'chown 10001:1000 /reports; chmod 770 /reports'
nvidia-smi --query-gpu=name,memory.total,memory.used,memory.free,driver_version --format=csv \
  > "$root/local-models/reports/gpu-before.csv"
docker run --rm --user 10001:10001 --read-only --cap-drop ALL \
  --security-opt no-new-privileges --pids-limit 64 --memory 128m \
  --network "$network" \
  --volume "$root/local-models/runtime/capabilities-smoke:/smoke:ro" \
  --volume "$model_root/private/token:/private/token:ro" \
  --volume "$root/local-models/reports:/reports" \
  --entrypoint /smoke alpine:3.21 \
  --addr local-models:9090 --token-file /private/token \
  --report /reports/verification-compose-capabilities.json
docker exec "$container" python -c \
  'import socket; s=socket.socket(); s.settimeout(2); assert s.connect_ex(("1.1.1.1",443)) != 0; s.close()'
nvidia-smi --query-gpu=name,memory.total,memory.used,memory.free,driver_version --format=csv \
  > "$root/local-models/reports/gpu-after.csv"
printf 'Authenticated main Compose capabilities and isolation verified: %s/local-models/reports\n' "$root"

#!/usr/bin/env bash
set -euo pipefail
root=${1:-/srv/storage/butler/agents/local-models}
mode=${2:-embedding}
testname=TestActualLocalRetrievalQuality
report=verification-retrieval-quality.json
if [[ "$mode" == hybrid ]]; then testname=TestActualHybridRetrievalQuality;report=verification-hybrid-quality.json;elif [[ "$mode" != embedding ]];then exit 2;fi
[[ "$root" == /srv/storage/butler/agents/* && "$root" != *..* ]] || exit 2
image=python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1
network=butler-local-models-quality
service=butler-local-models-quality-inference
postgres=butler-local-models-quality-postgres
mkdir -p "$root/.cache/go-tmp" "$root/runtime" "$root/reports" "$root/data/quality-postgres"
docker network inspect "$network" >/dev/null 2>&1 || docker network create --internal "$network" >/dev/null
if ! docker inspect "$postgres" >/dev/null 2>&1;then
  docker run --rm --user 0:0 --volume "$root/data/quality-postgres:/data" --entrypoint sh pgvector/pgvector:pg17 -c 'chown postgres:postgres /data; chmod 700 /data'
  docker run -d --name "$postgres" --network "$network" --network-alias semantic-postgres --memory 512m --pids-limit 64 \
    --env POSTGRES_USER=butler --env POSTGRES_PASSWORD=local-model-fixture-only --env POSTGRES_DB=semantic_test \
    --volume "$root/data/quality-postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 >/dev/null
fi
for attempt in $(seq 1 30);do
  if docker exec "$postgres" pg_isready -U butler -d semantic_test >/dev/null 2>&1;then break;fi
  sleep 1
done
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --volume "$root/current:/src:ro" \
  --volume "$root/.cache:/cache" --volume "$root/runtime:/runtime" --workdir /src \
  --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env BUTLER_TEST_SEMANTIC_POSTGRES_URL=postgres://butler:local-model-fixture-only@semantic-postgres:5432/semantic_test?sslmode=disable \
  --entrypoint sh golang:1.27.2-bookworm -c 'set -eu; go test -race -count=1 ./internal/memory/semantic ./internal/memory/service; CGO_ENABLED=0 go test -c -o /runtime/semantic.test ./internal/memory/semantic; CGO_ENABLED=0 go build -o /runtime/local-models ./apps/local-models'
cleanup(){ docker stop --time 15 "$service" >/dev/null 2>&1 || true; }
trap cleanup EXIT
nvidia-smi --query-gpu=name,memory.total,memory.used,memory.free,driver_version --format=csv > "$root/reports/quality-gpu-before.csv"
docker run -d --rm --init --name "$service" --user 10001:10001 --network "$network" \
  --gpus all --read-only --memory 8g --memory-swap 8g --pids-limit 256 --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /tmp:rw,nosuid,nodev,size=256m,uid=10001,gid=10001 \
  --volume "$root/runtime:/runtime:ro" --volume "$root/data:/data:ro" --volume "$root/private:/private:ro" \
  --volume "$root/reports:/reports" --volume "$root/current:/src:ro" \
  --volume "$root/current/apps/local-models/model-worker.py:/app/model-worker.py:ro" \
  --volume "$root/current/deploy/local-models/models.json:/app/models.json:ro" \
  --env BUTLER_LOCAL_MODELS_TOKEN_FILE=/private/token --env BUTLER_LOCAL_MODELS_CATALOG=/app/models.json \
  --env BUTLER_LOCAL_MODELS_IDLE_SECONDS=30 --env NVIDIA_DRIVER_CAPABILITIES=compute,utility \
  --entrypoint /runtime/local-models "$image" >/dev/null
for attempt in $(seq 1 30);do
  if docker exec "$service" python -c 'import urllib.request;urllib.request.urlopen("http://127.0.0.1:8080/health",timeout=2)' >/dev/null 2>&1;then break;fi
  sleep 1
done
docker exec --workdir /src/internal/memory/semantic \
  --env BUTLER_TEST_SEMANTIC_POSTGRES_URL=postgres://butler:local-model-fixture-only@semantic-postgres:5432/semantic_test?sslmode=disable \
  --env BUTLER_TEST_LOCAL_MODELS_ENDPOINT=127.0.0.1:9090 --env BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE=/private/token \
  --env "BUTLER_TEST_SEMANTIC_REPORT=/reports/$report" \
  "$service" /runtime/semantic.test -test.run "^${testname}$" -test.v -test.timeout 10m
docker logs "$service" > "$root/reports/service-retrieval-quality.log" 2>&1
cleanup
nvidia-smi --query-gpu=name,memory.total,memory.used,memory.free,driver_version --format=csv > "$root/reports/quality-gpu-after.csv"
printf 'Quality artifact: %s/reports/%s\n' "$root" "$report"

#!/usr/bin/env bash
set -euo pipefail
root="${1:-/srv/storage/butler/agents/local-models}"
if [[ ! "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ ]] || [[ "$root" == *..* ]]; then
 printf '%s\n' 'owned lab root required' >&2; exit 1
fi
# Coordinate the evaluation catalog and shared CPU worker with the operator.
# No deployment, cloud calls or main PostgreSQL changes occur here.
mkdir -p "$root/reports/decisions"
docker run --rm --network none --user 0:0 --volume "$root/reports/decisions:/out" \
 --entrypoint sh golang:1.27.2-bookworm -c 'chown 10001:10001 /out; chmod 755 /out'
docker run --rm --user "$(id -u):$(id -g)" --network none \
 --volume "$root/current:/src:ro" --volume "$root/.cache:/cache" --workdir /src \
 --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --entrypoint sh golang:1.27.2-bookworm -c 'go test -c -o /cache/decisions.test ./internal/decisions'
attached=false
if ! docker inspect butler-local-models-quality-postgres --format '{{json .NetworkSettings.Networks}}' | python3 -c 'import json,sys;sys.exit(0 if "butler-lab_local-inference" in json.load(sys.stdin) else 1)'; then
 docker network connect --alias semantic-postgres butler-lab_local-inference butler-local-models-quality-postgres
 attached=true
fi
sampler=''
cleanup() {
 if [[ -n "$sampler" ]]; then kill "$sampler" 2>/dev/null || true; wait "$sampler" || true; fi
 if $attached; then docker network disconnect butler-lab_local-inference butler-local-models-quality-postgres; fi
}
trap cleanup EXIT
nvidia-smi --query-gpu=memory.used,memory.free --format=csv,noheader
docker exec butler-lab-ollama-1 ollama ps
python3 "$root/current/deploy/local-models/sample-decisions.py" > "$root/reports/decisions-worker-cpu.jsonl" &
sampler=$!
docker run --rm --name butler-owned-decision-quality --user 10001:10001 --network butler-lab_local-inference \
 --read-only --tmpfs /tmp:size=256m,mode=1777 \
 --volume "$root/current:/src:ro" --volume "$root/.cache/decisions.test:/runner:ro" \
 --volume "$root/private/token:/private/token:ro" --volume "$root/reports/decisions:/reports" \
 --volume "$root/reports/decisions-worker-cpu.jsonl:/cpu-samples.jsonl:ro" \
 --workdir /src/internal/decisions \
 --env BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler:local-model-fixture-only@semantic-postgres:5432/semantic_test?sslmode=disable \
 --env BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE=/private/token \
 --env BUTLER_TEST_DECISION_REPORT=/reports/verification-decisions.json \
 --env BUTLER_TEST_DECISION_CPU_SAMPLES=/cpu-samples.jsonl \
 --entrypoint /runner golang:1.27.2-bookworm -test.v -test.run '^TestActualRussianDecisionComparison$' -test.timeout 10m
nvidia-smi --query-gpu=memory.used,memory.free --format=csv,noheader
docker exec butler-lab-ollama-1 ollama ps

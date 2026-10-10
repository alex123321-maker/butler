#!/usr/bin/env bash
set -euo pipefail
root="${1:-/srv/storage/butler/agents/local-models}"
if [[ ! "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ ]] || [[ "$root" == *..* ]]; then
 printf '%s\n' 'owned lab root required' >&2; exit 1
fi
# Coordinate the one persistent CPU/GPU worker before running this public fixture.
# Only an isolated test DB joins its internal network. Main DB/Compose is untouched.
mkdir -p "$root/reports/extraction"
docker run --rm --network none --user 0:0 --volume "$root/reports/extraction:/out" \
 --entrypoint sh golang:1.27.2-bookworm -c 'chown 10001:10001 /out; chmod 755 /out'
docker run --rm --user "$(id -u):$(id -g)" --network none \
 --volume "$root/current:/src:ro" --volume "$root/.cache:/cache" --workdir /src \
 --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --entrypoint sh golang:1.27.2-bookworm -c 'go test -c -o /cache/extraction.test ./internal/memory/extraction'
attached=false
if ! docker inspect butler-local-models-quality-postgres --format '{{json .NetworkSettings.Networks}}' | python3 -c 'import json,sys;sys.exit(0 if "butler-lab_local-inference" in json.load(sys.stdin) else 1)'; then
 docker network connect --alias semantic-postgres butler-lab_local-inference butler-local-models-quality-postgres
 attached=true
fi
cleanup() { if $attached; then docker network disconnect butler-lab_local-inference butler-local-models-quality-postgres; fi; }
trap cleanup EXIT
nvidia-smi --query-gpu=memory.used,memory.free --format=csv,noheader
docker exec butler-lab-ollama-1 ollama ps
docker run --rm --name butler-owned-extraction-quality --user 10001:10001 --network butler-lab_local-inference \
 --read-only --tmpfs /tmp:size=256m,mode=1777 \
 --volume "$root/current:/src:ro" --volume "$root/.cache/extraction.test:/runner:ro" \
 --volume "$root/private/token:/private/token:ro" --volume "$root/reports/extraction:/reports" \
 --workdir /src/internal/memory/extraction \
 --env BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler:local-model-fixture-only@semantic-postgres:5432/semantic_test?sslmode=disable \
 --env BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE=/private/token \
 --env BUTLER_TEST_EXTRACTION_REPORT=/reports/verification-structured-extraction.json \
 --entrypoint /runner golang:1.27.2-bookworm -test.v -test.run '^TestActualStructuredExtractionQuality$' -test.timeout 6m
nvidia-smi --query-gpu=memory.used,memory.free --format=csv,noheader
docker exec butler-lab-ollama-1 ollama ps

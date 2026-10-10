#!/usr/bin/env bash
set -euo pipefail
root="${1:-/srv/storage/butler/agents/local-models}"
if [[ ! "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9/_-]+$ ]] || [[ "$root" == *..* ]]; then
 printf '%s\n' 'owned lab root required' >&2; exit 1
fi
# Only this fixture binary and disposable PostgreSQL join the existing internal
# network. No inference service, shared Compose or main DB is created/modified.
mkdir -p "$root/owned-proof"
docker run --rm --user "$(id -u):$(id -g)" --network butler-local-models-quality \
 --volume "$root/current:/src" --volume "$root/.cache:/cache" --workdir /src \
 --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 --entrypoint sh golang:1.27.2-bookworm -c 'set -eu; gofmt -w internal/memory/semantic; go test -c ./internal/memory/semantic -o /cache/semantic-owned.test'
docker run --rm --user 0:0 --network none --volume "$root/owned-proof:/report" --entrypoint sh golang:1.27.2-bookworm -c 'chown 10001:10001 /report; chmod 700 /report'
nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits
docker exec butler-lab-ollama-1 ollama ps
attached=false
if ! docker inspect butler-local-models-quality-postgres --format '{{json .NetworkSettings.Networks}}' | python3 -c 'import json,sys;sys.exit(0 if "butler-lab_local-inference" in json.load(sys.stdin) else 1)'; then
 docker network connect --alias semantic-postgres butler-lab_local-inference butler-local-models-quality-postgres
 attached=true
fi
cleanup() { if $attached; then docker network disconnect butler-lab_local-inference butler-local-models-quality-postgres; fi; }
trap cleanup EXIT
docker run --rm --name butler-local-models-owned-proof --user 10001:10001 --network butler-lab_local-inference \
 --read-only --tmpfs /tmp:rw,size=64m --volume "$root/current:/src:ro" \
 --volume "$root/.cache/semantic-owned.test:/app/test:ro" --volume "$root/private/token:/private/token:ro" \
 --volume "$root/owned-proof:/report" --workdir /src/internal/memory/semantic \
 --env BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler:local-model-fixture-only@semantic-postgres:5432/semantic_test?sslmode=disable \
 --env BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE=/private/token \
 --env BUTLER_TEST_OWNED_SEMANTIC_REPORT=/report/verification-owned-semantic.json \
 --entrypoint /app/test golang:1.27.2-bookworm -test.run '^TestActualOwnedSemanticAuthority$' -test.v -test.timeout 5m
nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits
docker exec butler-lab-ollama-1 ollama ps

#!/usr/bin/env bash
set -euo pipefail
root=${BUTLER_MOBILE_TEST_ROOT:-/srv/storage/butler/agents/mobile-memory-20261020}
network=butler-mobile-memory-tests
pg=butler-mobile-memory-tests-pg
runner=butler-mobile-memory-tests-runner
mkdir -p "$root/postgres" "$root/reports"
docker network inspect "$network" >/dev/null 2>&1 || docker network create --internal --subnet 10.237.231.0/24 "$network" >/dev/null
if ! docker inspect "$pg" >/dev/null 2>&1; then
 docker run -d --pull never --name "$pg" --network "$network" --memory 512m --cpus 1 -v "$root/postgres:/var/lib/postgresql/data" -e POSTGRES_USER=butler -e POSTGRES_PASSWORD=synthetic-test -e POSTGRES_DB=memory_test pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=64 -c max_wal_size=128MB >/dev/null
fi
for i in {1..30};do docker exec "$pg" pg_isready -U butler -d memory_test >/dev/null && break;sleep 1;done
if ! docker inspect butler-mobile-memory-tests-redis >/dev/null 2>&1;then
 docker run -d --pull never --name butler-mobile-memory-tests-redis --network "$network" --memory 128m --cpus 1 redis:7.4-alpine >/dev/null
fi
if docker inspect "$runner" >/dev/null 2>&1;then
 test "$(docker inspect -f '{{.State.Running}}' "$runner")" = false
 docker rm "$runner" >/dev/null
fi
docker create --name "$runner" --network "$network" --cpus 3 --memory 4g \
 -v "$root/current:/src:ro" -v "$root/reports:/reports" -v "$root/.cache:/artifacts" \
 -v /srv/storage/butler/.cache/go-build:/cache -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v /srv/storage/butler/.cache/go-tmp:/tmp \
 -v /srv/storage/butler/agents/local-models/private/token:/private-local-token:ro -w /src \
 -e BUTLER_MOBILE_RETENTION_FINAL="${BUTLER_MOBILE_RETENTION_FINAL:-0}" -e GOCACHE=/cache -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_test?sslmode=disable" \
 -e BUTLER_TEST_SEMANTIC_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_test?sslmode=disable" \
 -e BUTLER_TEST_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_test?sslmode=disable" \
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_test?sslmode=disable" \
 -e BUTLER_TEST_COMPLETION_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_test?sslmode=disable" \
 -e BUTLER_TEST_INITIATIVE_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_test?sslmode=disable" \
 -e BUTLER_TEST_GOALS_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_test?sslmode=disable" \
 -e BUTLER_TEST_REDIS_URL=redis://butler-mobile-memory-tests-redis:6379/0 \
 -e BUTLER_TEST_OWNED_INDEX_REAL_TOKEN_FILE=/private-local-token \
 -e BUTLER_MEMORY_READ_APP_BINARY=/artifacts/tool-broker \
 -e BUTLER_TOOL_DISCOVERY_APP_BINARY=/artifacts/tool-broker \
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker.test \
 golang:1.27.2-bookworm bash -eu -c '
 go build -o /artifacts/tool-broker ./apps/tool-broker > /reports/build.log 2>&1
 go test -c -o /artifacts/broker.test ./apps/tool-broker/internal/broker >> /reports/build.log 2>&1
 go test -c -o /artifacts/mobile.test ./internal/mobile >> /reports/build.log 2>&1
 go test -race -json -p 1 -count=1 -timeout 10m -skip "^TestRetainedMobileActualBackupPhase$" -run "TestRetainedMobile|TestRetentionErases|TestMalformedMessageSearch|TestMobileTaskHistory|TestToolSchemaProjection|TestAutomaticIndex|TestMobileAgentBrokerSearchRememberNoiseAndDeletionIntegration|TestActualPinnedOwnedIndexRuntimeIntegration" ./apps/orchestrator/internal/orchestrator ./internal/memory/indexing ./internal/mobile ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/app > /reports/targeted.jsonl 2>&1
 if [ "$BUTLER_MOBILE_RETENTION_FINAL" = 1 ]; then
 go test -race -json -p 1 -count=1 -timeout 10m ./internal/memory/catalog ./internal/memory/chunks ./internal/memory/profile ./internal/memory/episodic ./internal/memory/reader ./internal/mobile ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/registry > /reports/regression.jsonl 2>&1
 go test -race -json -p 1 -count=1 -timeout 5m -run "TestModelSupervisorCurrentPinnedSourceAndAuthorityIntegration|TestMobile" ./internal/memory/jobs ./apps/orchestrator/internal/initiative >> /reports/regression.jsonl 2>&1
 go test -race -json -p 1 -count=1 -timeout 5m -run "TestMemoryReadModeZero|TestMemoryReadDispatched|TestMobileAgentBroker|TestMemoryEvalCollectorFullCorrelationIntegration" ./apps/orchestrator/internal/app > /reports/app-regression.jsonl 2>&1
 else
 go test -race -json -p 1 -count=1 -timeout 15m ./internal/memory/catalog ./internal/memory/chunks ./internal/memory/profile ./internal/memory/episodic ./internal/memory/experience ./internal/memory/jobs ./internal/memory/indexing ./internal/memory/semantic ./internal/memory/transcript ./internal/memory/readcontrol ./internal/memory/reader ./internal/mobile ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/initiative ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/registry > /reports/regression.jsonl 2>&1
 go test -race -json -p 1 -count=1 -timeout 15m -run "TestMemory|TestMobile|TestPrivateRead|TestStrictRead|TestSearchOpen|TestCredentialFragment|TestBootstrap" ./apps/orchestrator/internal/app > /reports/app-regression.jsonl 2>&1
 fi
 go vet ./internal/memory/catalog ./internal/memory/chunks ./internal/memory/profile ./internal/memory/episodic ./internal/memory/experience ./internal/memory/jobs ./internal/memory/indexing ./internal/memory/semantic ./internal/memory/transcript ./internal/memory/readcontrol ./internal/memory/reader ./internal/mobile ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/initiative ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/registry > /reports/vet.log 2>&1
 CGO_ENABLED=0 go build -o /artifacts/orchestrator ./apps/orchestrator > /reports/release-build.log 2>&1
 CGO_ENABLED=0 go build -o /artifacts/tool-broker ./apps/tool-broker >> /reports/release-build.log 2>&1
 CGO_ENABLED=0 go build -o /artifacts/migrator ./apps/migrator >> /reports/release-build.log 2>&1
 ' >/dev/null
docker network connect butler-lab_local-inference "$runner"
docker start -a "$runner"
code=$(docker inspect -f '{{.State.ExitCode}}' "$runner")
exit "$code"

#!/usr/bin/env bash
# Isolated PostgreSQL + actual Broker acceptance. Existing images/modules only.
set -euo pipefail
root=${1:-/srv/storage/butler/agents/issue77-20261016}
[[ "$root" == /srv/storage/butler/agents/issue77-* ]] || exit 2
[[ -f "$root/current/go.mod" && -d /srv/storage/butler/.cache/go-mod ]] || exit 2
mkdir -p "$root/reports" "$root/.cache/go-build" "$root/.cache/go-tmp"
network=issue86-20261016
pg=issue86-20261016-pg
if ! docker network inspect "$network" >/dev/null 2>&1; then
 docker network create --internal --subnet 10.237.86.0/24 "$network" >/dev/null
fi
if ! docker inspect "$pg" >/dev/null 2>&1; then
 docker run --pull never -d --name "$pg" --network "$network" \
  -e POSTGRES_USER=butler -e POSTGRES_PASSWORD=synthetic-test -e POSTGRES_DB=memory_read_test pgvector/pgvector:pg17 >/dev/null
fi
for i in {1..30}; do
 docker exec "$pg" pg_isready -U butler -d memory_read_test >/dev/null && break
 sleep 1
done
runner=(docker run --pull never --rm --network "$network" --cpus 2 --memory 3g
 -v "$root/current:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/gomod -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local
 -e BUTLER_TEST_MEMORY_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/memory_read_test?sslmode=disable"
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/cache/broker.test
 -e BUTLER_MEMORY_READ_APP_BINARY=/cache/tool-broker
 -e BUTLER_MEMORY_CANDIDATE_APP_BINARY=/cache/tool-broker
 golang:1.27.2-bookworm)
"${runner[@]}" go test -c -o /cache/broker.test ./apps/tool-broker/internal/broker > "$root/reports/read-build.log" 2>&1
"${runner[@]}" go build -o /cache/tool-broker ./apps/tool-broker >> "$root/reports/read-build.log" 2>&1
"${runner[@]}" go test -json -p 1 -count=1 -run 'TestStrictRead|TestPrivateRead|TestSearchOpen|TestCredentialFragmentMetadata|TestMemoryRead|TestMemoryCandidateDispatched|TestDelegationAdmissionTokenIsNotAuthority' \
 ./internal/memory/readcontrol ./internal/memory/reader ./internal/actionauth ./internal/config ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/app > "$root/reports/read-acceptance.jsonl" 2>&1
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$root/reports/read-acceptance.jsonl"; then
 printf 'Required read checks skipped or failed\n' >&2; exit 1
fi
"${runner[@]}" go vet ./internal/memory/readcontrol ./internal/memory/reader ./internal/actionauth ./internal/config ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/app > "$root/reports/read-vet.log" 2>&1
printf 'PASS: bounded catalog reads, actual Broker, candidate/delegation regression and vet; logs %s/reports\n' "$root"

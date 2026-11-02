#!/usr/bin/env bash
# Isolated proof/capacity regression. No production SQL, provider or channel.
set -euo pipefail
root=${1:?owned browser authority test root}
[[ "$root" =~ ^/srv/storage/butler/agents/browser-authority-isolation-[0-9]{8}$ ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
mkdir -p "$root/reports" "$root/.cache/go-tmp"
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
pg="butler-browser-authority-pg-$run"
created=false
cleanup() {
 if $created; then
  docker logs "$pg" > "$root/reports/postgres-$run.log" 2>&1 || true
  docker rm -f "$pg" >/dev/null
 fi
}
trap cleanup EXIT
docker run -d --name "$pg" --network none --memory 1g --cpus 1 --pids-limit 128 \
 --tmpfs /var/lib/postgresql/data:rw,size=768m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=authority_test "$pg_image" \
 -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB >/dev/null
created=true
for i in $(seq 1 30); do docker exec "$pg" pg_isready -U postgres -d authority_test >/dev/null 2>&1 && break; sleep 1; done
docker exec "$pg" pg_isready -U postgres -d authority_test
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_COMMUNICATION_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable'
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable'
 -e BUTLER_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable'
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable')
report="$root/reports/authority-$run.jsonl"
printf 'snapshot=%s go=%s postgres=%s\n' "$src" "$go_image" "$pg_image"
docker run "${args[@]}" "$go_image" go test -c -race -o /cache/authority-broker.test ./apps/tool-broker/internal/broker
if ! docker run "${args[@]}" -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/cache/authority-broker.test "$go_image" \
 go test -race -p 1 -count=1 -json -timeout 8m ./internal/storage/postgres ./apps/orchestrator/internal/session ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app \
 -run '^TestCommunicationAuthority|^TestAuthorityRetry|^TestDurableLeaseFence|^TestExecute(RetainsOwned|StopsInFlight|RenewsLease)|^TestCurrentRequestScopeCoreBrokerIntegration$' > "$report"; then
 grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$report" || tail -30 "$report"; exit 1
fi
if grep -q '"Action":"skip"' "$report"; then grep '"Action":"skip"' "$report"; exit 1; fi
for required in TestCommunicationAuthorityReserveWhileBusinessPoolSaturatedIntegration TestCommunicationAuthorityPoolAllocationIntegration TestDurableLeaseFenceTakeoverIntegration TestCurrentRequestScopeCoreBrokerIntegration; do
 grep -q "\"Test\":\"$required\"" "$report"
done
docker run "${args[@]}" "$go_image" go vet ./internal/storage/postgres ./apps/orchestrator/internal/session ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app
printf 'Required PG/race authority isolation/cancellation/cap checks passed without skips; vet PASS. report=%s\n' "$report"

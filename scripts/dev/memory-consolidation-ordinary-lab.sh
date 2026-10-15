#!/usr/bin/env bash
# Actual ordinary input/Flow/Broker/read/finalization, controlled model only.
set -euo pipefail
base=${1:?isolated issue89 root required}
label=${2:?fresh proof label required}
[[ "$base" =~ ^/srv/storage/butler/agents/issue89-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
tag=butler-consolidation-ordinary-${base##*/}-$label
network=$tag-net
pg=$tag-pg
created_pg=false
created_network=false
cleanup() {
 $created_pg && docker logs --tail 40 "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet "${BUTLER_CONSOLIDATION_ORDINARY_SUBNET:-10.253.190.0/28}" "$network" > "$proof/network-id"
created_network=true
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 512m --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=butler_consolidation_ordinary_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60); do
 docker exec "$pg" pg_isready -U butler -d butler_consolidation_ordinary_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
docker run --rm --pull never --network "$network" --cpus 2 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler@postgres:5432/butler_consolidation_ordinary_test?sslmode=disable \
 -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker \
 golang:1.27.2-bookworm bash -c 'go build -o /proof/tool-broker ./apps/tool-broker && go test -json -race -p 1 -count=1 -timeout 5m -run "^TestConsolidation" ./internal/memory/consolidation ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app' \
 > "$proof/events.jsonl" 2>&1
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestConsolidationOrdinaryTaskReadsActivatedDATAIntegration") and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/events.jsonl" >/dev/null
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/events.jsonl" > "$proof/summary.json"
sha256sum "$proof/source.sha256" "$proof/events.jsonl" "$proof/tool-broker" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

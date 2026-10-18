#!/usr/bin/env bash
# Controlled #87 mechanics, actual PostgreSQL and production Broker; no inference.
set -euo pipefail
base=${1:?isolated issue87 root}
label=${2:?fresh proof label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue87-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
network=butler-issue87-$label-net
pg=butler-issue87-$label-pg
created_pg=false
created_net=false
cleanup(){
 $created_pg && docker logs --tail 50 "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet "${BUTLER_ADAPTIVE_SUBNET:-10.237.87.0/24}" "$network" > "$proof/network-id"
created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_adaptive_test \
 pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for i in $(seq 1 60);do docker exec "$pg" pg_isready -U butler -d memory_adaptive_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1;done
runner=(docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_adaptive_test?sslmode=disable'
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/cache/broker-adaptive.test -e BUTLER_MEMORY_READ_APP_BINARY=/cache/tool-broker-adaptive
 golang:1.27.2-bookworm)
"${runner[@]}" bash -eu -c 'go test -c -o /cache/broker-adaptive.test ./apps/tool-broker/internal/broker; go build -o /cache/tool-broker-adaptive ./apps/tool-broker; sha256sum /cache/broker-adaptive.test /cache/tool-broker-adaptive > /proof/binaries.sha256' > "$proof/build.log" 2>&1
"${runner[@]}" bash -eu -c 'go test -json -race -p 1 -count=1 -timeout 8m -run "TestAdaptive|TestReported|TestTransparent|TestDecodeAnswer|TestPlanRejects|TestBuildBundleAdaptive|TestMemoryRead|TestSearchOpen|TestCandidateAcceptedPreferenceScopedOrdinaryContext" ./internal/memory/adaptive ./internal/memory/readcontrol ./internal/memory/reader ./internal/memory/service ./internal/config ./apps/orchestrator/internal/app > /proof/mechanics.jsonl' > "$proof/runner.log" 2>&1
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/mechanics.jsonl";then exit 1;fi
while IFS= read -r name;do
 [[ -z "$name" || "$name" == \#* ]] && continue
 jq -s -e --arg name "$name" 'any(.[];.Action=="pass" and .Test==$name) and all(.[];.Test!=$name or (.Action!="fail" and .Action!="skip"))' "$proof/mechanics.jsonl" >/dev/null
done < "$src/scripts/dev/memory-adaptive-required-tests.txt"
"${runner[@]}" go vet ./internal/memory/adaptive ./internal/memory/readcontrol ./internal/memory/reader ./internal/memory/service ./internal/config ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app > "$proof/vet.log" 2>&1
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failed:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/mechanics.jsonl" > "$proof/summary.json"
sha256sum "$proof/source.sha256" "$proof/binaries.sha256" "$proof/mechanics.jsonl" "$proof/vet.log" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

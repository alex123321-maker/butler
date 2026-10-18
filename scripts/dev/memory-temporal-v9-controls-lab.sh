#!/usr/bin/env bash
# Fresh disposable V9 PG only; no authentication, provider or retained-store writes.
set -euo pipefail
base=${1:?isolated issue88 root}
label=${2:?fresh controlled label}
history_label=${3:?existing immutable read-only history label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+$ && "$label" =~ ^[A-Za-z0-9_-]+$ && "$history_label" =~ ^[A-Za-z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
history=$base/reports/$history_label/history
test -s "$history/history.json"
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
sha256sum "$history/history.json" > "$proof/history.sha256"
network=butler-issue88-$label-history-net
pg=butler-issue88-$label-history-pg
created_network=false;created_pg=false
cleanup(){
 if $created_pg;then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true;docker rm -f "$pg" >/dev/null 2>&1 || true;created_pg=false;fi
 if $created_network;then docker network rm "$network" >/dev/null 2>&1 || true;created_network=false;fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Docker rejects overlapping existing networks; cleanup only removes resources
# whose exact IDs were created by this fresh invocation.
docker network create --internal --subnet 10.237.210.0/28 "$network" > "$proof/network-id"
created_network=true
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 1g --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=temporal_v9_history_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=64MB -c max_connections=24 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60);do
 docker exec "$pg" pg_isready -h 127.0.0.1 -U butler -d temporal_v9_history_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
status=0
docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -v "$history:/history:ro" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -v /srv/storage/butler/agents/issue88-native-v7-20261018/reports/closed-history-v1/history:/closed-history:ro \
 -e BUTLER_CLOSED_TEMPORAL_V7_HISTORY_TEST_ROOT=/closed-history \
 -v /srv/storage/butler/agents/issue88-native-v8-20261018/reports/closed-history-v1/history:/closed8:ro -e BUTLER_LATEST_CLOSED_OWNER_HISTORY_TEST_ROOT=/closed8 \
 -e BUTLER_TEMPORAL_V9_NAMESPACE_CONTROL=1 -e BUTLER_TEMPORAL_V9_HISTORY_TEST_ROOT=/history \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/temporal_v9_history_test?sslmode=disable' \
 golang:1.27.2-bookworm bash -eu -c 'go test -json -race -p 1 -count=1 -timeout 10m -run "^Test(ClosedTemporalV7(RetainsProductNativeIndependence|RejectsConsistentInvalidSequence|FrozenHistoryRejectsForgeries)|LatestClosedOwnerV8(FrozenHistoryRejectsForgeries|RetainsIndependentProductPopulation)|ClosedV9PinsRejectDifferentProfile|TemporalV9(FactsContractBoundary|CurrentDayCarryOneUseIntegration|NamespaceClaimOneUseIntegration|FrozenHistoryRejectsForgeries)|TemporalV6WorkerRejectsOwnerControlIdentity|ConsolidationV3ClosureRetainsFailedCollector)$" ./apps/orchestrator/internal/app > /proof/events.jsonl
 go vet ./apps/orchestrator/internal/app > /proof/vet.log' > "$proof/runner.log" 2>&1 || status=$?
if [[ "$status" == 0 ]];then
 python3 "$src/scripts/dev/memory-temporal-auth-recovery-controls.py" > "$proof/auth-recovery-controls.log" 2>&1 || status=$?
 python3 "$src/scripts/dev/memory-temporal-auth-recovery-lab.py" "$base" "$label" "$pg" "$network" > "$proof/auth-recovery-lab.log" 2>&1 || status=$?
fi
cleanup
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
for name in ClosedTemporalV7RetainsProductNativeIndependence ClosedTemporalV7RejectsConsistentInvalidSequence ClosedTemporalV7FrozenHistoryRejectsForgeries LatestClosedOwnerV8FrozenHistoryRejectsForgeries LatestClosedOwnerV8RetainsIndependentProductPopulation ClosedV9PinsRejectDifferentProfile TemporalV9FactsContractBoundary TemporalV9CurrentDayCarryOneUseIntegration TemporalV9NamespaceClaimOneUseIntegration TemporalV9FrozenHistoryRejectsForgeries TemporalV6WorkerRejectsOwnerControlIdentity ConsolidationV3ClosureRetainsFailedCollector;do
 jq -s -e --arg name "Test$name" 'any(.[];.Action=="pass" and .Test==$name) and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/events.jsonl" >/dev/null
done
printf '{"controlled_only":true,"native_calls":0,"auth_imported":false,"client_cpu":1,"client_memory_bytes":2147483648,"postgres_cpu":1,"postgres_memory_bytes":1073741824,"separate_pg":true,"seven_native_stores_modified":false,"compiler_and_modules":"cached_offline"}\n' > "$proof/environment.json"
sha256sum "$proof/source.sha256" "$proof/history.sha256" "$proof/events.jsonl" "$proof/vet.log" "$proof/environment.json" "$proof/auth-recovery-controls.log" "$proof/auth-recovery-lab-results.json" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf 'V9 original witnesses and actual PG one-use carry passed; native0/auth0.\n'

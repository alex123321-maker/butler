#!/usr/bin/env bash
# Real-PG one-use positive-carry control using the already frozen collector.
# Private PG only: no auth, provider service, main or retained history mutation.
set -euo pipefail
base=${1:?isolated issue88 root}
label=${2:?fresh controlled proof label}
native_label=${3:?native proof containing compiled collector}
[[ "$base" =~ ^/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+$ && "$label" =~ ^[A-Za-z0-9_-]+$ && "$native_label" =~ ^[A-Za-z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
native=$base/reports/$native_label
test -x "$native/collector.test"
test -s "$native/prepared"
sha256sum --check --status "$native/prepared-pins.sha256"
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres"
network=butler-issue88-$label-net
pg=butler-issue88-$label-pg
created_pg=false
created_net=false
cleanup(){
 $created_pg && docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet "${BUTLER_TEMPORAL_CARRY_SUBNET:-10.237.198.0/24}" "$network" > "$proof/network-id"
created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_temporal_carry_test \
 pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=32 > "$proof/postgres-id"
created_pg=true
for i in $(seq 1 60);do docker exec "$pg" pg_isready -U butler -d memory_temporal_carry_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1;sleep 1;done
docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$native:/native:ro" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_temporal_carry_test?sslmode=disable' \
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/native/broker.test \
 golang:1.27.2-bookworm /native/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /native/collector.test -test.v -test.timeout 3m -test.run '^TestTemporal(V5PositiveCarryOneUseIntegration|CollectorFactsThenBothOriginalsIntegration)$' > "$proof/carry.jsonl" 2> "$proof/runner.log"
! grep -q '"Action":"skip"\|"Action":"fail"' "$proof/carry.jsonl"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestTemporalV5PositiveCarryOneUseIntegration")' "$proof/carry.jsonl" >/dev/null
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestTemporalCollectorFactsThenBothOriginalsIntegration")' "$proof/carry.jsonl" >/dev/null
sha256sum "$native/collector.test" "$native/source.sha256" "$proof/carry.jsonl" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf 'POSITIVE_CARRY_ACTUAL_PG_RACE_PASS\n'

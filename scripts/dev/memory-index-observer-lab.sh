#!/usr/bin/env bash
# Controlled local-index observer diagnosis: real PG, delayed local test backend,
# no external model request, private token, main database or auth rotation.
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
docker network create --internal --subnet "${BUTLER_INDEX_OBSERVER_SUBNET:-10.237.187.0/24}" "$network" > "$proof/network-id"
created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_index_observer_test \
 pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for i in $(seq 1 60);do docker exec "$pg" pg_isready -U butler -d memory_index_observer_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1;done
runner=(docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_index_observer_test?sslmode=disable'
 golang:1.27.2-bookworm)
pattern='^Test(SlowLocalIndexRetainsBothCurrentnessObservers|LocalIndexObserverCommitDelayCancelsWithoutRefund|SlowLocalIndexCurrentnessCancellationRejectsLateBackend)Integration$'
"${runner[@]}" go test -json -race -p 1 -count=1 -timeout 4m -run "$pattern" ./internal/memory/indexing > "$proof/observers.jsonl" 2> "$proof/runner.log"
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/observers.jsonl";then exit 1;fi
for name in TestSlowLocalIndexRetainsBothCurrentnessObserversIntegration TestLocalIndexObserverCommitDelayCancelsWithoutRefundIntegration TestSlowLocalIndexCurrentnessCancellationRejectsLateBackendIntegration;do
 jq -s -e --arg name "$name" 'any(.[];.Action=="pass" and .Test==$name)' "$proof/observers.jsonl" >/dev/null
done
"${runner[@]}" go vet ./internal/memory/indexing > "$proof/vet.log" 2>&1
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failed:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/observers.jsonl" > "$proof/summary.json"
sha256sum "$proof/source.sha256" "$proof/observers.jsonl" "$proof/vet.log" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

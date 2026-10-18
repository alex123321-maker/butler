#!/usr/bin/env bash
# Sealed controlled-provider load with real PG/ordinary Flow/local IPC. No cloud auth.
set -euo pipefail
base=${1:?isolated issue91 mixed root}; label=${2:?fresh report label}
mode=${3:-mixed}
case "$mode" in
 mixed) test_name=TestMemoryCapacityCombinedMixedIntegration; cold=0 ;;
 cold) test_name=TestMemoryCapacityPGSharedBufferColdIntegration; cold=1 ;;
 *) exit 2 ;;
esac
[[ "$base" =~ ^/srv/storage/butler/agents/issue91-mixed-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label; test ! -e "$proof"; umask 077
available=$(df -B1 --output=avail "$base" | tail -n1 | tr -d ' ')
[[ "$available" =~ ^[0-9]+$ && "$available" -ge 21474836480 ]] || exit 1
model=${BUTLER_CAPACITY_LOCAL_MODEL_IP:?operator-pinned local IPv4}
[[ "$model" =~ ^172\.(1[6-9]|2[0-9]|3[01])\.[0-9]+\.[0-9]+$ ]] || exit 2
token=${BUTLER_CAPACITY_LOCAL_TOKEN_FILE:-/srv/storage/butler/agents/local-models/private/token}
[[ "$token" == /srv/storage/butler/agents/local-models/private/* && -f "$token" ]] || exit 2
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
sha256sum "$src/docs/ai/memory-capacity-protocol.md" > "$proof/prospective-protocol.sha256"
{ date -u --iso-8601=ns; uname -a; lscpu; free -b; df -B1 /srv/storage; cat /proc/uptime; } > "$proof/hardware.txt"
printf 'deployment_no_race; client=2CPU/2GiB; pg=2CPU/1GiB; pool_max=2; p95=1500ms; wall=5000ms\n' > "$proof/build-profile.txt"
network=butler-issue91-$label-mixed-net; pg=butler-issue91-$label-mixed-pg; client=butler-issue91-$label-mixed-client
created_pg=false; created_net=false; created_client=false; stats_pid=''; restart_pid=''
cleanup(){
 [[ -z "$restart_pid" ]] || { kill "$restart_pid" 2>/dev/null || true; wait "$restart_pid" 2>/dev/null || true; restart_pid=''; }
 [[ -z "$stats_pid" ]] || { kill "$stats_pid" 2>/dev/null || true; wait "$stats_pid" 2>/dev/null || true; stats_pid=''; }
 if $created_client; then docker inspect "$client" --format '{{json .State}}' > "$proof/client-state.json" || true; docker rm -f "$client" >/dev/null 2>&1 || true; created_client=false; fi
 if $created_pg; then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true; docker rm -f "$pg" >/dev/null 2>&1 || true; created_pg=false; fi
 if $created_net; then docker network rm "$network" >/dev/null 2>&1 || true; created_net=false; fi
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet 10.237.208.0/24 "$network" > "$proof/network-id"; created_net=true
docker run --pull never -d --name "$pg" --network "$network" --cpus 2 --memory 1g --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_mixed_test \
 pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id"; created_pg=true
pg_ip=$(docker inspect "$pg" --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
for i in $(seq 1 60); do docker exec "$pg" pg_isready -h127.0.0.1 -U butler -d memory_mixed_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
docker exec "$pg" psql -U butler -d memory_mixed_test -Atqc 'SELECT version(); CREATE EXTENSION IF NOT EXISTS vector; SELECT extversion FROM pg_extension WHERE extname=$$vector$$' > "$proof/versions.txt"
docker inspect "$pg" --format '{{json .HostConfig}}' | jq '{CpuPeriod,CpuQuota,NanoCpus,Memory,MemorySwap,PidsLimit}' > "$proof/pg-limits.json"
docker exec "$pg" cat /sys/fs/cgroup/cpu.stat > "$proof/pg-cpu-initial.txt"
docker inspect butler-lab-local-models-1 --format '{{json .HostConfig}}' | jq '{CpuPeriod,CpuQuota,NanoCpus,Memory,MemorySwap,PidsLimit,DeviceRequests}' > "$proof/shared-local-limits.json"
(while true; do
 date -u +%Y-%m-%dT%H:%M:%S.%NZ
 docker stats --no-stream --format '{{json .}}' "$client" "$pg" butler-lab-local-models-1 2>/dev/null || true
 for target in "$client" "$pg"; do
  { date -u +%Y-%m-%dT%H:%M:%S.%NZ; printf '%s\n' "$target"; docker exec "$target" sh -c 'for item in cpu.stat memory.events memory.peak; do printf "%s\n" "$item"; cat "/sys/fs/cgroup/$item"; done' 2>/dev/null || true; } >> "$proof/cgroup-samples.log"
 done
 docker exec "$pg" psql -U butler -d memory_mixed_test -Atqc 'SELECT json_build_object($$observed_at$$,clock_timestamp(),$$state$$,state,$$wait_type$$,wait_event_type,$$wait_event$$,wait_event,$$count$$,count(*),$$blocked$$,count(*) FILTER(WHERE cardinality(pg_blocking_pids(pid))>0)) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() GROUP BY state,wait_event_type,wait_event' >> "$proof/pg-waits.jsonl" 2>/dev/null || true
 sleep 1
done) > "$proof/resources.log" & stats_pid=$!
docker create --pull never --name "$client" --network host --add-host "local-models:$model" --cpus 2 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -v "$token:/local-token:ro" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_MEMORY_CAPACITY_MIXED=1 -e "BUTLER_MEMORY_CAPACITY_COLD=$cold" -e "BUTLER_CAPACITY_TEST_NAME=$test_name" -e BUTLER_MEMORY_CAPACITY_REPORT=/proof -e BUTLER_MEMORY_SEMANTIC_ENABLED=true \
 -e BUTLER_MEMORY_LOCAL_SERVICE_ID=operator-pinned-service -e BUTLER_MEMORY_LOCAL_TOKEN_FILE=/local-token \
 -e "BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler@$pg_ip:5432/memory_mixed_test?sslmode=disable" \
 golang:1.27.2-bookworm bash -c 'go test -json -p 1 -count=1 -timeout 14m -run "^${BUTLER_CAPACITY_TEST_NAME}$" ./apps/orchestrator/internal/app > /proof/tests.jsonl 2>/proof/build.log; test_status=$?; go vet ./apps/orchestrator/internal/app > /proof/vet.log 2>&1; vet_status=$?; for item in cpu.stat memory.events memory.peak; do cat "/sys/fs/cgroup/$item" > "/proof/client-$item-final.txt"; done; printf "%s %s\n" "$test_status" "$vet_status" > /proof/exit-status; exit "$((test_status || vet_status))"' > "$proof/client-id"; created_client=true
docker inspect "$client" --format '{{json .HostConfig}}' | jq '{CpuPeriod,CpuQuota,NanoCpus,Memory,MemorySwap,PidsLimit}' > "$proof/client-limits.json"
if [[ "$mode" == cold ]]; then
 (
  test -f "$proof/postgres-id"
  pg_id=$(cat "$proof/postgres-id"); [[ "$pg_id" =~ ^[a-f0-9]{64}$ ]]
  until=$((SECONDS+400))
  while [[ ! -f "$proof/cold-ready.json" ]]; do
   [[ "$SECONDS" -lt "$until" ]] || exit 1
   sleep 1
  done
  docker exec "$client" cat /proof/cold-ready.json | jq -e '.database=="memory_mixed_test" and (.schema|test("^memory_[a-f0-9]{32}$")) and .stage=="after_actual_initial_local_index"' >/dev/null
  [[ "$(docker inspect "$pg_id" --format '{{.Name}}')" == "/$pg" ]]
  [[ "$(docker inspect "$pg_id" --format '{{.HostConfig.NanoCpus}} {{.HostConfig.Memory}}')" == '2000000000 1073741824' ]]
  [[ "$(docker inspect "$pg_id" --format '{{range .Mounts}}{{if eq .Destination "/var/lib/postgresql/data"}}{{.Source}}{{end}}{{end}}')" == "$proof/postgres" ]]
  [[ "$(docker inspect "$pg_id" --format '{{.HostConfig.NetworkMode}}')" == "$network" ]]
  docker inspect "$pg_id" --format '{{json .State}}' > "$proof/cold-pg-before.json"
  date -u --iso-8601=ns > "$proof/cold-restart-start.txt"
  docker restart --time 10 "$pg_id" > "$proof/cold-restarted-container-id"
  for i in $(seq 1 60); do docker exec "$pg_id" pg_isready -h127.0.0.1 -U butler -d memory_mixed_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
  docker inspect "$pg_id" --format '{{json .State}}' > "$proof/cold-pg-after.json"
  date -u --iso-8601=ns > "$proof/cold-restart-finish.txt"
  jq -n --arg id "$pg_id" '{restarted:true,owned_container_id:$id,database:"memory_mixed_test"}' > "$proof/cold-resumed.tmp"
  mv -- "$proof/cold-resumed.tmp" "$proof/cold-resumed.json"
 ) > "$proof/cold-restart.log" 2>&1 & restart_pid=$!
fi
docker start -a "$client" > "$proof/runner.log" 2>&1 || true
status=$(docker wait "$client")
if [[ -n "$restart_pid" ]]; then
 if [[ -f "$proof/cold-resumed.json" ]]; then wait "$restart_pid"; else kill "$restart_pid" 2>/dev/null || true; wait "$restart_pid" 2>/dev/null || true; status=1; fi
 restart_pid=''
fi
for item in cpu.stat memory.events memory.peak; do docker exec "$pg" cat "/sys/fs/cgroup/$item" > "$proof/pg-$item-final.txt"; done
cleanup
# Only named report files are made operator-readable; the retained PG data stays
# unchanged. The report root itself remains private mode0700.
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm \
 bash -c 'find /proof -maxdepth 1 -type f -exec chown "$1:$2" {} +' -- "$(id -u)" "$(id -g)"
find "$proof" -maxdepth 1 -type f ! -name evidence.sha256 ! -name complete -print0 | sort -z | xargs -0 sha256sum > "$proof/evidence.sha256"
[[ "$status" == 0 ]] || exit 1
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/tests.jsonl"; then exit 1; fi
jq -s -e --arg name "$test_name" 'any(.[];.Action=="pass" and .Test==$name)' "$proof/tests.jsonl" >/dev/null
if [[ "$mode" == cold ]]; then
 jq -e '.pass and .phase=="closed_cold" and (.warm|length)==100 and (.first|length)==1 and (."matched-warm"|length)==1 and .native_cloud_calls==0 and .warm_p95_ms<=1500 and .first[0].Status=="completed" and ."matched-warm"[0].Status=="completed" and .private_pg_restart.same_10000_chunks and .private_pg_restart.same_reservations and .private_pg_restart.catalog_current' "$proof/combined-mixed.json" >/dev/null
else
 jq -e '.pass and .phase=="closed" and (.warm|length)==100 and (.mixed|length)==100 and .native_cloud_calls==0 and .warm_p95_ms<=1500 and .mixed_p95_ms<=1500 and .foreground_maintenance_overlaps>=10 and .actual_query_index_ipc_overlaps>=1 and .maintenance.maintenance_ok and .maintenance.index_done and .foreign_denied and .stable_source_current' "$proof/combined-mixed.json" >/dev/null
fi
printf 'PASS\n' > "$proof/complete"

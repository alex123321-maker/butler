#!/usr/bin/env bash
# Real App.New/Run + its HTTP Memory Doctor; no native/auth or main attachments.
set -euo pipefail
base=${1:?isolated issue92 bootstrap root}; label=${2:?fresh report label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-bootstrap-[A-Za-z0-9_-]+$ && "$label" =~ ^[A-Za-z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label; test ! -e "$proof"; umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
ip -j route > "$proof/routes-before.json"
docker network ls -q | xargs -r docker network inspect --format '{{.Name}} {{range .IPAM.Config}}{{.Subnet}} {{end}}' > "$proof/subnets-before.txt"
network=butler-issue92-$label-bootstrap-net; pg=butler-issue92-$label-bootstrap-pg; redis=butler-issue92-$label-bootstrap-redis; client=butler-issue92-$label-bootstrap-client
for name in "$pg" "$redis" "$client"; do ! docker container inspect "$name" >/dev/null 2>&1 || exit 2; done
created_pg=false; created_redis=false; created_net=false; created_client=false
cleanup(){
 if $created_client; then docker rm -f "$client" >/dev/null 2>&1 || true; created_client=false; fi
 if $created_redis; then docker logs "$redis" > "$proof/redis.log" 2>&1 || true; docker rm -f "$redis" >/dev/null 2>&1 || true; created_redis=false; fi
 if $created_pg; then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true; docker rm -f "$pg" >/dev/null 2>&1 || true; created_pg=false; fi
 if $created_net; then docker network rm "$network" >/dev/null 2>&1 || true; created_net=false; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# Operator verifies route overlap prospectively; Docker also rejects IPAM overlap.
docker network create --internal --subnet 10.237.214.0/28 "$network" > "$proof/network-id"; created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 1g --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_bootstrap_test \
 pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id"; created_pg=true
docker run --pull never -d --name "$redis" --network "$network" --network-alias redis --cpus .25 --memory 128m --pids-limit 64 \
 redis:7-alpine redis-server --save '' --appendonly no --maxmemory 64mb --maxmemory-policy noeviction > "$proof/redis-id"; created_redis=true
for i in $(seq 1 60); do docker exec "$pg" pg_isready -h127.0.0.1 -U butler -d memory_bootstrap_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
for i in $(seq 1 30); do docker exec "$redis" redis-cli ping >/dev/null 2>&1 && break; [[ "$i" != 30 ]] || exit 1; sleep 1; done
status=0; created_client=true
docker run --rm --pull never --name "$client" --network "$network" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_BOOTSTRAP_SMOKE=1 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_bootstrap_test?sslmode=disable' \
 -e BUTLER_TEST_MEMORY_REDIS_URL='redis://redis:6379/0' -e BUTLER_TEST_DOCTOR_BROKER_BINARY=/proof/tool-broker -e BUTLER_MEMORY_BOOTSTRAP_PROOF=/proof/bootstrap.json \
 golang:1.27.2-bookworm bash -eu -c '
 go build -p 1 -o /proof/tool-broker ./apps/tool-broker
 go test -json -race -p 1 -count=1 -timeout 5m -run "^TestMemoryRealBootstrapDoctorIntegration$" ./apps/orchestrator/internal/app > /proof/tests.jsonl
 go test -json -race -p 1 -count=1 -timeout 1m -run "^TestOperatorDoctorCommandIdentitySurvivesReplayWithoutCrossRunCollision$" ./apps/orchestrator/internal/orchestrator > /proof/identity-tests.jsonl
 go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator > /proof/vet.log 2>&1
 ' > "$proof/runner.log" 2>&1 || status=$?
cleanup
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm \
 bash -c 'find /proof -path /proof/postgres -prune -o -exec chown -h "$1:$2" {} +' -- "$(id -u)" "$(id -g)"
find "$proof" -maxdepth 1 -type f ! -name evidence.sha256 ! -name complete -print0 | sort -z | xargs -0 sha256sum > "$proof/evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
! grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/tests.jsonl"
! grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/identity-tests.jsonl"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestOperatorDoctorCommandIdentitySurvivesReplayWithoutCrossRunCollision")' "$proof/identity-tests.jsonl" >/dev/null
for name in TestMemoryRealBootstrapDoctorIntegration TestMemoryRealBootstrapDoctorIntegration/default_off TestMemoryRealBootstrapDoctorIntegration/experience_context_without_jobs TestMemoryRealBootstrapDoctorIntegration/selected_requested TestMemoryRealBootstrapDoctorIntegration/unsupported_profile TestMemoryRealBootstrapDoctorIntegration/rollback_default_off; do
 jq -s -e --arg name "$name" 'any(.[];.Action=="pass" and .Test==$name)' "$proof/tests.jsonl" >/dev/null
done
jq -e '.pass and .native_calls==0 and .auth_imported==false and .foreground_model_attempts==0 and .background_model_attempts==0 and .system_settings_rows==0 and .worker_control=="disabled" and .controlled_diagnostic_runtime and (.cases|length)==5 and (.retained_completed_reports==(if .experience_qualified then 4 else 2 end)) and .doctor_tool_rows==.retained_completed_reports and .doctor_distinct_runs==.retained_completed_reports' "$proof/bootstrap.json" >/dev/null
sha256sum -c "$proof/source.sha256" >/dev/null
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failed:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/tests.jsonl" "$proof/identity-tests.jsonl" > "$proof/summary.json"
printf '{"client_cpu":1,"client_memory_bytes":2147483648,"postgres_cpu":1,"postgres_memory_bytes":1073741824,"redis_cpu":0.25,"redis_memory_bytes":134217728,"published_ports":0,"auth_imported":false,"native_calls":0,"main_changed":false}\n' > "$proof/environment.json"
sha256sum "$proof/summary.json" "$proof/environment.json" >> "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"

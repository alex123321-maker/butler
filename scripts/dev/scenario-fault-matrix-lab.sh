#!/usr/bin/env bash
# Disposable owning converter result/delivery faults. No model/provider network.
set -euo pipefail
root=${1:?isolated agent root required}
mode=${2:-tail}
commit=${3:?exact source commit required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ && "$mode" =~ ^(authority|authority-all|tail|baseline|regression)$ && "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/go.mod" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/scenario-fault-$run_id"
prefix="butler-scenario-fault-$run_id"
network="$prefix-net"
pg="$prefix-pg"
runtime="$prefix-converter"
socket="$prefix-socket"
umask 077
mkdir -p "$proof/bin" "$proof/private/postgres" "$proof/private/runtime" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
created_network=false
created_pg=false
created_runtime=false
created_socket=false
cleanup() {
  $created_runtime && docker logs --tail 80 "$runtime" > "$proof/private/runtime.log" 2>&1 || true
  $created_runtime && docker rm -f "$runtime" >/dev/null 2>&1 || true
  $created_pg && docker logs --tail 80 "$pg" > "$proof/private/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
  $created_socket && docker volume rm "$socket" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
printf 'Private scenario fault proof: %s\n' "$proof"
printf '%s\n' "$commit" > "$proof/source-commit.txt"
# Complete read-only source bytes, including schemas/data and test helpers.
(cd "$source_dir"; find . -type f -not -path './.cache/*' -print0 | sort -z | xargs -0 sha256sum) > "$proof/source-manifest.sha256"
sha256sum "$proof/source-manifest.sha256" > "$proof/source-manifest-hash.txt"
go_image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
pg_image=$(docker image inspect pgvector/pgvector:pg17 --format '{{.Id}}')
runtime_image=$(docker image inspect alpine:3.21 --format '{{.Id}}')
lscpu --json > "$proof/hardware-cpu.json"
cpu_hash=$(sha256sum "$proof/hardware-cpu.json" | cut -d' ' -f1)
ram_kib=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)
printf '{"go_image":"%s","postgres_image":"%s","runtime_base_image":"%s","go":"1.27.2","kernel":"%s","logical_cpus":%s,"host_ram_kib":%s,"cpu_manifest_sha256":"%s","build_memory_bytes":2147483648,"fixture_memory_bytes":1610612736,"converter_memory_bytes":134217728,"build_cpus":2,"fixture_cpus":2,"postgres_cpus":1,"converter_cpus":1,"provider":"deterministic_scripted","actual_native_calls":0,"channel":"loopback_http_simulator","weights":"not_used"}\n' \
  "$go_image" "$pg_image" "$runtime_image" "$(uname -r)" "$(getconf _NPROCESSORS_ONLN)" "$ram_kib" "$cpu_hash" > "$proof/hardware.json"
build=(docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none
  --memory 2g --pids-limit 192 --cpus 2 --security-opt no-new-privileges:true
  --volume "$source_dir:/src:ro" --volume "$proof:/proof" --volume /srv/storage/butler/.cache:/cache --workdir /src
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp)
"${build[@]}" golang:1.27.2-bookworm bash -c '
  set -euo pipefail
  go test -c -race -o /proof/bin/app.test ./apps/orchestrator/internal/app
  CGO_ENABLED=0 go build -trimpath -o /proof/bin/tool-broker ./apps/tool-broker
  CGO_ENABLED=0 go build -trimpath -o /proof/bin/tool-converter ./apps/tool-converter
  go build -trimpath -o /proof/bin/test2json cmd/test2json
  go vet ./apps/orchestrator/internal/app
' > "$proof/build-vet.log" 2>&1
sha256sum "$proof/bin/"* > "$proof/binaries.sha256"
converter_hash=$(sha256sum "$proof/bin/tool-converter" | cut -d' ' -f1)
profile='{"dependency":{"id":"butler-converter-ir","version":"1.0.0","sha256":"'"$converter_hash"'"},"checker":"butler.delimited-json.checks.v1"}'
printf '%s' "$profile" > "$proof/runtime-profile.json"
printf '%s' 'synthetic-scenario-converter-token-000000000000000000' > "$proof/private/runtime/token"
chmod 555 "$proof/bin/"*
chmod 755 "$proof/bin"
chmod 444 "$proof/private/runtime/token"
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --runtime runc --name "$pg" --network "$network" --network-alias scenario-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=scenario-fixture --env POSTGRES_DB=butler_scenario_fault_test \
  --volume "$proof/private/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=32 -c work_mem=4MB -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$pg" pg_isready -h 127.0.0.1 -U fixture -d butler_scenario_fault_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
docker exec "$pg" psql -q -v ON_ERROR_STOP=1 -U fixture -d butler_scenario_fault_test \
  -c 'CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public' >/dev/null
if [[ "$mode" == authority || "$mode" == authority-all ]]; then
  authority_filter='^TestClaim(RunDataGuardDerivesOnlyCurrentDatabaseOwnershipIntegration|GrantExpiryDuringPrivateSourceReadProofRollsBackIntegration)$'
  if [[ "$mode" == authority-all ]]; then authority_filter='.'; fi
  docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
    --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
    --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2 \
    --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
    --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:scenario-fixture@scenario-postgres:5432/butler_scenario_fault_test?sslmode=disable' \
    golang:1.27.2-bookworm go test -json -race -count=1 -timeout=120s \
    -run "$authority_filter" ./internal/actionauth > "$proof/tests.jsonl"
  "${build[@]}" golang:1.27.2-bookworm go vet ./internal/actionauth > "$proof/authority-vet.log" 2>&1
  sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
  authority_required=(TestClaimRunDataGuardDerivesOnlyCurrentDatabaseOwnershipIntegration
    TestClaimGrantExpiryDuringPrivateSourceReadProofRollsBackIntegration)
  for fault in plain_rpc forged_caller expired_lease superseded_lease stale_task current_source_denied; do
    authority_required+=("TestClaimRunDataGuardDerivesOnlyCurrentDatabaseOwnershipIntegration/$fault")
  done
  for required_test in "${authority_required[@]}"; do
    test "$(awk -v needle="\"Test\":\"$required_test\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$proof/tests.jsonl")" = 1
  done
  ! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
  printf '{"test_exit":0,"mode":"%s","actual_native_calls":0}\n' "$mode" > "$proof/summary.json"
  printf 'PASS: actual-PG same-TX source proof, race/vet, no native calls.\n'
  exit 0
fi
docker volume create "$socket" >/dev/null
created_socket=true
docker run --rm --runtime runc --network none --volume "$socket:/run/butler-converter" alpine:3.21 \
  sh -ec 'chown 10001:10001 /run/butler-converter; chmod 700 /run/butler-converter'
docker run -d --runtime runc --name "$runtime" --user 10001:10001 --network none --read-only \
  --memory 128m --pids-limit 64 --cpus 1 --cap-drop ALL --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,noexec,nosuid,nodev,size=4m,uid=10001,gid=10001,mode=0700 \
  --volume "$proof/bin/tool-converter:/usr/local/bin/tool-converter:ro" \
  --volume "$proof/private/runtime/token:/run/secrets/converter-runtime:ro" \
  --volume "$socket:/run/butler-converter" \
  --env "BUTLER_CONVERTER_PROFILE_JSON=$profile" --env BUTLER_HTTP_ADDR=127.0.0.1:8080 \
  --env BUTLER_CONVERTER_UNIX_SOCKET=/run/butler-converter/runtime.sock \
  --env BUTLER_CONVERTER_RUNTIME_TOKEN_FILE=/run/secrets/converter-runtime \
  --entrypoint /usr/local/bin/tool-converter alpine:3.21 >/dev/null
created_runtime=true
for attempt in $(seq 1 40); do
  if docker exec "$runtime" wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 40 ]] || exit 1
  sleep .1
done
test "$(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}|{{.HostConfig.NetworkMode}}|{{.Config.User}}' "$runtime")" = 'true|none|10001:10001'
docker exec "$runtime" sh -ec '
  test "$(id -u)" = 10001
  test -S /run/butler-converter/runtime.sock
  test "$(stat -c %a /run/butler-converter/runtime.sock)" = 600
  test ! -e /src; test ! -e /var/run/docker.sock; test ! -e /srv/storage
  grep -q "^CapEff:[[:space:]]*0000000000000000$" /proc/self/status
  grep -q "^NoNewPrivs:[[:space:]]*1$" /proc/self/status
  test -z "${BUTLER_POSTGRES_URL:-}${BUTLER_REDIS_URL:-}${OPENAI_API_KEY:-}${BUTLER_OWNER_CONTROL_TOKEN:-}"
' > "$proof/runtime-isolation.txt"
filter='^TestScenarioFaultConverterResultDeliveryContainerE2E$'
if [[ "$mode" == baseline ]]; then
  filter='^TestConverterDeveloper(RealBrokerWorkers|AutomaticParent)ContainerE2E$'
fi
if [[ "$mode" == regression ]]; then
  filter='^(TestScenarioFaultConverterResultDeliveryContainerE2E|TestConverterDeveloper(RealBrokerWorkers|AutomaticParent)ContainerE2E|TestConverterContinuation.*Integration)$'
fi
set +e
docker run --rm --runtime runc --user 10001:10001 --network "$network" --read-only \
  --memory 1536m --pids-limit 192 --cpus 2 --cap-drop ALL --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,nosuid,nodev,size=256m,uid=10001,gid=10001,mode=0700 \
  --volume "$source_dir:/src:ro" --volume "$proof/bin:/proof/bin:ro" \
  --volume "$socket:/run/butler-converter:ro" --volume "$proof/private/runtime/token:/run/secrets/converter-runtime:ro" \
  --workdir /src/apps/orchestrator/internal/app \
  --env BUTLER_CONVERTER_DEVELOPER_E2E=1 --env BUTLER_CONVERTER_BROKER_BINARY=/proof/bin/tool-broker \
  --env "BUTLER_CONVERTER_PROFILE_JSON=$profile" --env BUTLER_CONVERTER_RUNTIME_TOKEN_FILE=/run/secrets/converter-runtime \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:scenario-fixture@scenario-postgres:5432/butler_scenario_fault_test?sslmode=disable' \
  --entrypoint /proof/bin/test2json golang:1.27.2-bookworm \
  -t -p github.com/butler/butler/apps/orchestrator/internal/app /proof/bin/app.test \
  -test.v=test2json -test.timeout=300s -test.run="$filter" > "$proof/tests.jsonl"
test_exit=$?
set -e
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
required=(TestScenarioFaultConverterResultDeliveryContainerE2E
  TestScenarioFaultConverterResultDeliveryContainerE2E/restart_before_send
  TestScenarioFaultConverterResultDeliveryContainerE2E/explicit_absence
  TestScenarioFaultConverterResultDeliveryContainerE2E/ack_commit_failure)
if [[ "$mode" == baseline ]]; then
  required=(TestConverterDeveloperRealBrokerWorkersContainerE2E TestConverterDeveloperAutomaticParentContainerE2E)
fi
if [[ "$mode" == regression ]]; then
  required+=(TestConverterDeveloperRealBrokerWorkersContainerE2E
    TestConverterDeveloperAutomaticParentContainerE2E
    TestConverterContinuationQueueOnceRestartIntegration
    TestConverterContinuationAuthoritySourceAndBudgetDenialIntegration
    TestConverterContinuationQueueFailureRollsBackIntegration
    TestConverterContinuationDeadlineAfterWaitIntegration
    TestConverterContinuationPrivateClaimAndRecoveryIntegration
    TestConverterContinuationLateAtomicFinalDeniesDeletedOriginalIntegration
    TestConverterContinuationReadinessRequiresOwnedTypedSchemaIntegration
    TestConverterContinuationPrivateLoadRechecksExpiryAfterSourceWaitIntegration
    TestConverterContinuationMaintenanceSkipsLiveParentBeforeRunLocksIntegration)
fi
# test2json emits compact records in fixed encoding. Exact named pass counts
# deny missing/renamed tests, zero-test selection, duplicate execution or skips.
for required_test in "${required[@]}"; do
  count=$(awk -v needle="\"Test\":\"$required_test\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$proof/tests.jsonl")
  [[ "$count" == 1 ]] || { printf 'Required scenario did not pass exactly once: %s\n' "$required_test" >&2; test_exit=1; }
done
if grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"; then test_exit=1; fi
if ! grep -Eq '"Action":"pass","Package":"github.com/butler/butler/apps/orchestrator/internal/app","Elapsed":' "$proof/tests.jsonl"; then test_exit=1; fi
test -z "$(docker exec "$runtime" ls -A /tmp)"
printf '{"test_exit":%s,"mode":"%s","actual_native_calls":0,"channel":"loopback_http_simulator","full_release_chain":"pending","runtime_tmp_empty":true}\n' "$test_exit" "$mode" > "$proof/summary.json"
[[ "$test_exit" == 0 ]]
printf 'PASS: owning converter result/delivery fault tail, race/vet, no native calls.\n'

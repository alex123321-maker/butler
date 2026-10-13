#!/usr/bin/env bash
# Separate V5 owning release chain; native High is sealed behind the V5 bridge.
set -euo pipefail
root=${1:?isolated agent root required}
mode=${2:-baseline}
commit=${3:?exact source commit required}
[[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v5-high-20261013 && "$mode" =~ ^(baseline|chain|high_v5|diagnose_absence)$ && "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/go.mod" ]] || exit 2
high=false
if [[ "$mode" == high_v5 ]]; then
    [[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v5-high-20261013 && "${BUTLER_SCENARIO_CHAIN_V5_HIGH_SEALED_ENTRY:-}" == 1 ]] || exit 2
    window=/srv/storage/butler/native-acceptance-windows/issue33-high-release-chain-v5-20261013-baseline535
    chain_schema=fixture_chain_v5_high; chain_baseline=535; high_test=TestScenarioReleaseChainActualV5HighE2E
    high_env=BUTLER_SCENARIO_CHAIN_V5_HIGH
  high=true
  [[ -d "$window" && ! -L "$window" && ! -e "$window/native-started" && "$(cat "$window/release-ref")" == "$commit" && "$(cat "$window/source-ref")" == "$source_dir" ]] || exit 2
  prepared=$(cat "$root/prepared-proof")
  [[ "$prepared" == "$root/reports/chain-v5-high-prepare-"* && "$(cat "$root/prepared-release")" == "$commit" ]]
fi
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/release-chain-process-$run_id"
prefix="butler-release-chain-process-$run_id"
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
created_native=false
cleanup() {
  $created_native && docker rm -f "$prefix-native" >/dev/null 2>&1 || true
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
planning=deterministic_scripted; native_count=0
if $high; then planning=actual_native_High; native_count=null; fi
printf '{"go_image":"%s","postgres_image":"%s","runtime_base_image":"%s","go":"1.27.2","kernel":"%s","logical_cpus":%s,"host_ram_kib":%s,"cpu_manifest_sha256":"%s","build_memory_bytes":2147483648,"fixture_memory_bytes":1610612736,"converter_memory_bytes":134217728,"build_cpus":2,"fixture_cpus":2,"postgres_cpus":1,"converter_cpus":1,"provider":"%s","actual_native_calls":%s,"channel":"loopback_http_simulator","weights":"not_observed"}\n' \
  "$go_image" "$pg_image" "$runtime_image" "$(uname -r)" "$(getconf _NPROCESSORS_ONLN)" "$ram_kib" "$cpu_hash" "$planning" "$native_count" > "$proof/hardware.json"
build=(docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none
  --memory 2g --pids-limit 192 --cpus 2 --security-opt no-new-privileges:true
  --volume "$source_dir:/src:ro" --volume "$proof:/proof" --volume /srv/storage/butler/.cache:/cache --workdir /src
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp)
if $high; then
  sha256sum -c "$root/prepared-runtime.sha256" >/dev/null
  cp "$prepared/bin/"* "$proof/bin/"
else
"${build[@]}" "$go_image" bash -c '
  set -euo pipefail
  go test -c -race -o /proof/bin/app.test ./apps/orchestrator/internal/app
  CGO_ENABLED=0 go build -trimpath -o /proof/bin/tool-broker ./apps/tool-broker
  CGO_ENABLED=0 go build -trimpath -o /proof/bin/tool-converter ./apps/tool-converter
  go test -c -o /proof/bin/extension.test ./apps/tool-extension/internal/runtime
  go build -trimpath -o /proof/bin/test2json cmd/test2json
  go vet ./apps/orchestrator/internal/app
' > "$proof/build-vet.log" 2>&1
fi
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
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=scenario-fixture --env POSTGRES_DB=butler_release_chain_test \
  --volume "$proof/private/postgres:/var/lib/postgresql/data" "$pg_image" \
  -c shared_buffers=32MB -c max_connections=32 -c work_mem=4MB -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$pg" pg_isready -h 127.0.0.1 -U fixture -d butler_release_chain_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
docker exec "$pg" psql -q -v ON_ERROR_STOP=1 -U fixture -d butler_release_chain_test \
  -c 'CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public' >/dev/null
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
  --entrypoint /usr/local/bin/tool-converter "$runtime_image" >/dev/null
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
filter='^TestScenarioReleaseChain(OneVerifiedNotice|GenericProviderCurrentOwnership|ProviderStartFailureRetainsAdmission)ContainerE2E$'
if [[ "$mode" == diagnose_absence ]]; then filter='^TestScenarioReleaseChainOneVerifiedNoticeContainerE2E$/^explicit_absence$'; fi
if [[ "$mode" == baseline ]]; then filter='^TestScenarioReleaseChainOneVerifiedNoticeContainerE2E/baseline$'; fi
native_args=()
if $high; then
  filter="^$high_test\$"
  mkdir -p "$proof/private/native"; chmod 700 "$proof/private/native"
  docker run --rm --network none --volume "$proof/private/native:/private" alpine:3.21 sh -eu -c 'chown 10001:10001 /private; chmod 700 /private'
  import_file=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli/butler-native-import.json
  native_args=(--volume "$proof/private/native:/private" --volume "$import_file:/home/butler/.gemini/antigravity-cli/butler-native-import.json:ro"
    --env "${high_env}_ACCEPTANCE=1" --env "${high_env}_PRIVATE=/private" --env "${high_env}_IMPORT_FILE=/home/butler/.gemini/antigravity-cli/butler-native-import.json"
    --env "${high_env}_DEADLINE=$(cat "$window/deadline")"
    --env HTTPS_PROXY=http://antigravity-proxy:18080 --env HTTP_PROXY=http://antigravity-proxy:18080 --env NO_PROXY=localhost,127.0.0.1,scenario-postgres)
fi
run_verb=run
run_name=(--rm)
if $high; then run_verb=create; run_name=(--name "$prefix-native"); fi
set +e
docker "$run_verb" --runtime runc "${run_name[@]}" --user 10001:10001 --network "$network" --read-only \
  --memory 1536m --pids-limit 192 --cpus 2 --cap-drop ALL --security-opt no-new-privileges:true \
  --tmpfs /tmp:rw,nosuid,nodev,size=256m,uid=10001,gid=10001,mode=0700 \
  --volume "$source_dir:/src:ro" --volume "$proof/bin:/proof/bin:ro" \
  --volume "$socket:/run/butler-converter:ro" --volume "$proof/private/runtime/token:/run/secrets/converter-runtime:ro" \
  --workdir /src \
  --env BUTLER_SCENARIO_RELEASE_CHAIN_E2E=1 --env BUTLER_EXTENSION_E2E=1 --env BUTLER_EXTENSION_E2E_BINARY=/proof/bin/extension.test --env BUTLER_BROKER_E2E_BINARY=/proof/bin/tool-broker --env BUTLER_CONVERTER_DEVELOPER_E2E=1 --env BUTLER_CONVERTER_BROKER_BINARY=/proof/bin/tool-broker \
  --env "BUTLER_CONVERTER_PROFILE_JSON=$profile" --env BUTLER_CONVERTER_RUNTIME_TOKEN_FILE=/run/secrets/converter-runtime \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:scenario-fixture@scenario-postgres:5432/butler_release_chain_test?sslmode=disable' \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:scenario-fixture@scenario-postgres:5432/butler_release_chain_test?sslmode=disable' \
  "${native_args[@]}" \
  --entrypoint /proof/bin/test2json "$go_image" \
  -t -p github.com/butler/butler/apps/orchestrator/internal/app /proof/bin/app.test \
  -test.v=test2json -test.timeout="$(if $high; then printf 61m; else printf 420s; fi)" -test.run="$filter" > "$proof/tests.jsonl"
test_exit=$?
set -e
if $high; then
  [[ "$test_exit" == 0 ]]
  created_native=true
  docker network connect butler-lab_backend "$prefix-native"
  (cd "$source_dir"; find . -type f -not -path './.cache/*' -print0 | sort -z | xargs -0 sha256sum) | cmp -s "$root/prepared-source.sha256" -
  mkdir "$window/native-started"
  printf '%s\n' "$proof" > "$window/native-started/proof-ref"
  set +e
  docker start -a "$prefix-native" > "$proof/tests.jsonl" 2>&1
  test_exit=$?
  set -e
  pg_actual_image=$(docker inspect --format '{{.Image}}' "$pg")
  converter_actual_image=$(docker inspect --format '{{.Image}}' "$runtime")
  docker inspect --format '{"image_id":{{json .Image}},"postgres_image_id":"'"$pg_actual_image"'","converter_image_id":"'"$converter_actual_image"'","started_at":{{json .State.StartedAt}},"finished_at":{{json .State.FinishedAt}},"exit_code":{{.State.ExitCode}},"oom_killed":{{.State.OOMKilled}},"memory_limit_bytes":{{.HostConfig.Memory}}}' "$prefix-native" > "$proof/runtime-state.json"
  docker rm -f "$prefix-native" >/dev/null
  created_native=false
  docker run --rm --network none --volume "$proof/private/native:/private:ro" alpine:3.21 cat /private/scope.json > "$proof/scope.json" || test_exit=1
  # Independent content-free export from the owning fixture ledger, before
  # disposable PG cleanup. No prompt, tool args, profile or private grant rows.
  docker exec "$pg" psql -X -At -v ON_ERROR_STOP=1 -U fixture -d butler_release_chain_test -c "SELECT json_build_object('window',(SELECT json_build_object('native_baseline',native_baseline,'attempts',attempts,'prior_history_hash',prior_history_hash,'prerequisite_hash',prerequisite_hash,'expires_at',expires_at) FROM $chain_schema.fixture_chain_v5_high_window),'baseline_attempts',$chain_baseline,'total_attempts',(SELECT attempts FROM $chain_schema.fixture_chain_v5_high_window WHERE singleton),'new_attempts',(SELECT count(*) FROM $chain_schema.fixture_chain_v5_high_requests),'requests',COALESCE((SELECT json_agg(row_to_json(r) ORDER BY ordinal) FROM (SELECT ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,admitted_at,outcome,input_tokens,output_tokens,total_tokens,model_version,finished_at,elapsed_ms,failure_stage,diagnostic_code,final_count,terminal_count,completed_count,cancelled_count,timed_out_count,transport_error_count,finish_reason,stop_final_count,max_tokens_final_count,usage_observation_uncertain,version_observation_uncertain,finish_observation_uncertain FROM $chain_schema.fixture_chain_v5_high_requests) r),'[]'::json))" > "$proof/pg-ledger-witness.json" || test_exit=1
  docker exec "$pg" psql -X -At -v ON_ERROR_STOP=1 -U fixture -d butler_release_chain_test -c "SELECT COALESCE(json_agg(row_to_json(r) ORDER BY scenario),'[]'::json) FROM (SELECT scenario,payload_sha256,payload_json,created_at FROM $chain_schema.fixture_chain_v5_high_case_evidence) r" > "$proof/pg-product-witness.json" || test_exit=1
  docker exec "$pg" psql -X -At -v ON_ERROR_STOP=1 -U fixture -d butler_release_chain_test -c "SELECT COALESCE(json_agg(row_to_json(r) ORDER BY scenario),'[]'::json) FROM (SELECT scenario,payload_sha256,payload_json,created_at FROM $chain_schema.fixture_chain_v5_high_case_initial) r" > "$proof/pg-initial-witness.json" || test_exit=1
  sha256sum "$proof/scope.json" "$proof/pg-ledger-witness.json" "$proof/pg-product-witness.json" "$proof/pg-initial-witness.json" "$proof/runtime-state.json" > "$proof/native-evidence.sha256"
fi
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
required=(TestScenarioReleaseChainOneVerifiedNoticeContainerE2E TestScenarioReleaseChainOneVerifiedNoticeContainerE2E/baseline)
if [[ "$mode" == diagnose_absence ]]; then required=(TestScenarioReleaseChainOneVerifiedNoticeContainerE2E TestScenarioReleaseChainOneVerifiedNoticeContainerE2E/explicit_absence); fi
if $high; then required=("$high_test" "$high_test/normal_sent" "$high_test/restart_sent" "$high_test/explicit_absence" "$high_test/lost_ack" "$high_test/owner_stop"); fi
if [[ "$mode" == chain ]]; then
 for fault in broker_restart_after_admission explicit_absence ack_commit_failure child_stop_after_admission technical_failure_projection; do required+=("TestScenarioReleaseChainOneVerifiedNoticeContainerE2E/$fault"); done
 required+=(TestScenarioReleaseChainGenericProviderCurrentOwnershipContainerE2E TestScenarioReleaseChainProviderStartFailureRetainsAdmissionContainerE2E)
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
printf '{"test_exit":%s,"mode":"%s","actual_native_calls":%s,"channel":"loopback_http_simulator","planning":"%s","runtime_tmp_empty":true}\n' "$test_exit" "$mode" "$native_count" "$planning" > "$proof/summary.json"
[[ "$test_exit" == 0 ]]
printf 'PASS: actual promoted-task release chain and single semantic notification, race/vet, planning=%s.\n' "$planning"

#!/usr/bin/env bash
# Full96 v5 preparation does not read auth. Execute needs independent seal review.
set -euo pipefail
base=${1:?isolated issue87-native root}
mode=${2:?prepare or execute}
release=${3:?exact source release SHA}
[[ "$base" =~ ^/srv/storage/butler/agents/issue87-native-[a-zA-Z0-9_-]+$ && "$release" =~ ^[0-9a-f]{40}$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/native-v5
umask 077
offline=(docker run --rm --pull never --network none --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$base/runtime:/runtime" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local)
if [[ "$mode" == prepare ]];then
 test ! -e "$proof"
 mkdir -p "$proof" "$base/private" "$base/runtime" "$base/.cache/go-build" "$base/.cache/go-tmp"
 find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
 printf '%s\n' "$src" > "$proof/source-reference"
 # Only container identity/address are inspected. No environment or token bytes.
 docker inspect --format '{{json .NetworkSettings.Networks}}' butler-lab-local-models-1 | jq -e '.["butler-lab_local-inference"].IPAddress | select(test("^172[.]28[.]0[.][0-9]+$"))' > "$proof/local-ip.json"
 docker inspect --format '{{.Id}}' butler-lab-local-models-1 > "$proof/local-container-id"
 test "$(stat -c '%u %a' /srv/storage/butler/agents/local-models/private/token)" = '10001 600'
 "${offline[@]}" -w /src golang:1.27.2-bookworm bash -eu -c 'go build -o /runtime/test2json cmd/test2json
 go test -c -race -o /runtime/collector.test ./apps/orchestrator/internal/app
 go build -o /runtime/tool-broker ./apps/tool-broker' > "$proof/build.log" 2>&1
 # Exact nonroot runner: no Go module command, writable HOME or build cache.
 "${offline[@]}" --user "$(id -u):$(id -g)" -w /src/apps/orchestrator/internal/app \
  golang:1.27.2-bookworm /runtime/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /runtime/collector.test \
  -test.v -test.run '^TestAdaptiveV5(ScheduleAndFiniteBudget|ContractRejectsDayOwnerScheduleAndCarry)$' > "$proof/nonroot-runner-preflight.jsonl" 2> "$proof/nonroot-runner-preflight.log"
 jq -s -e 'all(.[];.Action!="skip" and .Action!="fail") and any(.[];.Action=="pass" and .Test=="TestAdaptiveV5ScheduleAndFiniteBudget") and any(.[];.Action=="pass" and .Test=="TestAdaptiveV5ContractRejectsDayOwnerScheduleAndCarry")' "$proof/nonroot-runner-preflight.jsonl" >/dev/null
 python3 "$src/scripts/dev/memory-adaptive-history.py" prepare "$base" > "$proof/history-safe.json"
 "${offline[@]}" --user "$(id -u):$(id -g)" -v "$proof:/proof" -w /src/apps/orchestrator/internal/app \
  -e BUTLER_ADAPTIVE_WRITE_PINS=1 -e BUTLER_ADAPTIVE_RELEASE="$release" -e BUTLER_ADAPTIVE_NATIVE_PROOF=/proof \
  -e BUTLER_MEMORY_READ_APP_BINARY=/runtime/tool-broker -e BUTLER_ADAPTIVE_TEST2JSON_BINARY=/runtime/test2json \
  golang:1.27.2-bookworm /runtime/collector.test -test.run '^TestMemoryAdaptiveWritePins$' -test.v > "$proof/freeze.log" 2>&1
 history_hash=$(sha256sum "$proof/history.json" | cut -d' ' -f1)
 python3 "$src/scripts/dev/memory-adaptive-history.py" check-current "$base" "$history_hash" > "$proof/current-after-build.json"
 sha256sum "$proof/source.sha256" "$proof/source-reference" "$base/runtime/collector.test" "$base/runtime/tool-broker" "$base/runtime/test2json" \
  "$proof/nonroot-runner-preflight.jsonl" "$proof/local-ip.json" "$proof/local-container-id" "$proof/history.json" "$proof/six-store-ledgers.json" \
  "$proof/historical-v4-full-ledger.json" "$proof/historical-v4-native-calls.json" "$proof/scheduled-before-auth.json" "$proof/shared-policy.json" "$proof/frozen-pins.json" > "$proof/prepared-pins.sha256"
 printf 'PASS\n' > "$proof/prepared"
 sha256sum "$proof/prepared-pins.sha256"
 exit 0
fi
[[ "$mode" == execute ]] || exit 2
reviewed=${4:?independently reviewed exact prepared-pins SHA256}
auth_binary=${5:?reviewed existing provider-auth diagnostic binary}
[[ "$reviewed" =~ ^[a-f0-9]{64}$ && "$auth_binary" == /srv/storage/butler/agents/*/reports/provider-auth-current ]] || exit 2
test -s "$proof/prepared"
test ! -e "$proof/execution-claimed"
test "$(jq -r .release "$proof/frozen-pins.json")" = "$release"
test "$(sha256sum "$proof/prepared-pins.sha256" | cut -d' ' -f1)" = "$reviewed"
sha256sum --check --status "$proof/prepared-pins.sha256"
sha256sum --check --status "$proof/source.sha256"
read -r prepared_src < "$proof/source-reference"
test "$src" = "$prepared_src"
test "$(docker inspect --format '{{.Id}}' butler-lab-local-models-1)" = "$(cat "$proof/local-container-id")"
local_ip=$(jq -r . "$proof/local-ip.json")
test "$(docker inspect --format '{{json .NetworkSettings.Networks}}' butler-lab-local-models-1 | jq -r '.["butler-lab_local-inference"].IPAddress')" = "$local_ip"
# Retain full denominator even if private PG, runner or auth setup fails.
(set -o noclobber; printf 'claimed\n' > "$proof/execution-claimed")
cp --no-clobber "$proof/scheduled-before-auth.json" "$proof/all-original-outcomes.json"
network=butler-issue87-native-v5-net
pg=butler-issue87-native-v5-pg
created_pg=false
cleanup(){
 local status=${1:-1}
 trap - EXIT
 if $created_pg;then
  docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
  docker stop "$pg" > "$proof/postgres-stopped" 2>&1 || true
 fi
 printf '%s\n' "$status" > "$proof/native-status"
 exit "$status"
}
trap 'cleanup $?' EXIT
docker network create --internal --subnet 10.237.203.0/24 "$network" > "$proof/network-id"
test ! -e "$base/postgres"
mkdir -p "$base/postgres"
docker run --pull never -d --name "$pg" --network "$network" --ip 10.237.203.2 --cpus 1 --memory 1g --pids-limit 128 \
 -v "$base/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_adaptive_test \
 pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=64MB > "$proof/postgres-id"
created_pg=true
for i in $(seq 1 60);do docker exec "$pg" pg_isready -h 127.0.0.1 -U butler -d memory_adaptive_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1;done
"${offline[@]}" --user "$(id -u):$(id -g)" -v "$proof:/proof:ro" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_ADAPTIVE_CHECK_PINS=1 -e BUTLER_ADAPTIVE_NATIVE_PROOF=/proof -e BUTLER_MEMORY_READ_APP_BINARY=/runtime/tool-broker \
 -e BUTLER_ADAPTIVE_TEST2JSON_BINARY=/runtime/test2json golang:1.27.2-bookworm /runtime/collector.test -test.v -test.run '^TestMemoryAdaptiveCheckPreparedPins$' > "$proof/pre-auth-pins-check.txt" 2>&1
exec 9>/srv/storage/butler/agents/native-provider-auth.lock
flock -n 9
history_hash=$(sha256sum "$proof/history.json" | cut -d' ' -f1)
python3 "$src/scripts/dev/memory-adaptive-history.py" check-current "$base" "$history_hash" > "$proof/pre-auth-current-ledgers.json"
private=$base/private/native-v5
handback(){
 local status=$?
 trap - EXIT
 if [[ ! -s "$private/auth-return.value" ]];then
  python3 "$src/scripts/dev/memory-adaptive-auth.py" recover "$base" native-v5 > "$proof/recover-safe.json" || cleanup 1
 fi
 python3 "$src/scripts/dev/memory-adaptive-auth.py" return "$base" native-v5 > "$proof/return-safe.json" || cleanup 1
 docker run --rm --pull never --network container:butler-lab-orchestrator-1 --env-file "$private/main-diagnose.env" -v "$auth_binary:/provider-auth:ro" golang:1.27.2-bookworm /provider-auth -action diagnose-cloudcode > "$private/main-diagnosis.json" || cleanup 1
 jq -e -f "$src/scripts/dev/memory-consolidation-main-auth.jq" "$private/main-diagnosis.json" >/dev/null || cleanup 1
 jq '{resolved,unsafe_refresh,expired:.after.expired,trace:[.trace[]|{stage,outcome,http_status}]}' "$private/main-diagnosis.json" > "$proof/main-auth-safe.json"
 python3 "$src/scripts/dev/memory-adaptive-auth.py" snapshot "$base" native-v5 > "$proof/main-snapshot-safe.json" || cleanup 1
 if [[ -s "$proof/measurements.json" ]];then
  python3 "$src/scripts/dev/memory-adaptive-outcomes.py" "$proof" > "$proof/outcome-export-safe.json" || cleanup 1
 fi
 cleanup "$status"
}
python3 "$src/scripts/dev/memory-adaptive-auth.py" clone "$base" native-v5 > "$proof/clone-safe.json"
trap handback EXIT
# Copy through a private pipe/mount, preserving UID10001/mode600 on the source.
docker run --rm --pull never --network none -v /srv/storage/butler/agents/local-models/private/token:/from:ro -v "$private:/to" \
 golang:1.27.2-bookworm sh -eu -c 'cp /from /to/local-token; chmod 600 /to/local-token; chown "$1:$2" /to/local-token' sh "$(id -u)" "$(id -g)"
date -u +%Y-%m-%dT%H:%M:%SZ > "$proof/started"
docker run --rm --pull never --network host --user "$(id -u):$(id -g)" --add-host "local-models:$local_ip" --cpus 2 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$base/runtime:/runtime:ro" -v "$proof:/proof" -v "$private:/private" -w /src/apps/orchestrator/internal/app \
 --env-file "$private/native.env" -e BUTLER_ADAPTIVE_NATIVE_V1=1 -e BUTLER_ADAPTIVE_NATIVE_PROOF=/proof \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@10.237.203.2:5432/memory_adaptive_test?sslmode=disable' \
 -e BUTLER_MEMORY_READ_APP_BINARY=/runtime/tool-broker -e BUTLER_ADAPTIVE_TEST2JSON_BINARY=/runtime/test2json -e BUTLER_MEMORY_SEMANTIC_ENABLED=true \
 -e BUTLER_MEMORY_LOCAL_SERVICE_ID=operator-pinned-service -e BUTLER_MEMORY_LOCAL_TOKEN_FILE=/private/local-token \
 -e HTTP_PROXY=http://127.0.0.1:18080 -e HTTPS_PROXY=http://127.0.0.1:18080 -e ALL_PROXY=http://127.0.0.1:18080 -e "NO_PROXY=localhost,127.0.0.1,local-models,$local_ip,10.237.203.2" \
 golang:1.27.2-bookworm /runtime/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /runtime/collector.test -test.run '^TestMemoryAdaptiveNativeComparison$' -test.timeout 210m -test.v > "$proof/runner.jsonl" 2> "$proof/runner.log"
date -u +%Y-%m-%dT%H:%M:%SZ > "$proof/collector-exited"

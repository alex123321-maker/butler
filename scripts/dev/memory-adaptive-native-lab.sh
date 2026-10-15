#!/usr/bin/env bash
# Offline source/binary freeze followed by one retained native comparison.
set -euo pipefail
base=${1:?isolated issue87-native root}
mode=${2:?prepare or execute}
release=${3:?exact release SHA}
[[ "$base" =~ ^/srv/storage/butler/agents/issue87-native-[a-zA-Z0-9_-]+$ && "$release" =~ ^[0-9a-f]{40}$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/native-v1
umask 077
mkdir -p "$base/private" "$base/runtime" "$base/.cache/go-build" "$base/.cache/go-tmp"
offline=(docker run --rm --pull never --network none --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$base/runtime:/runtime" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local)
if [[ "$mode" == prepare ]];then
 test ! -e "$proof"
 mkdir -p "$proof"
 "${offline[@]}" -w /src golang:1.27.2-bookworm bash -eu -c 'go test -c -o /runtime/collector.test ./apps/orchestrator/internal/app; go build -o /runtime/tool-broker ./apps/tool-broker' > "$proof/build.log" 2>&1
 # Neither provider credentials nor any network exist during the freeze.
 "${offline[@]}" --user "$(id -u):$(id -g)" -v "$proof:/proof" -w /src/apps/orchestrator/internal/app -e BUTLER_ADAPTIVE_WRITE_PINS=1 \
  -e BUTLER_ADAPTIVE_RELEASE="$release" -e BUTLER_ADAPTIVE_NATIVE_PROOF=/proof -e BUTLER_MEMORY_READ_APP_BINARY=/runtime/tool-broker \
  golang:1.27.2-bookworm /runtime/collector.test -test.run '^TestMemoryAdaptiveWritePins$' -test.v > "$proof/freeze.log" 2>&1
 sha256sum "$base/runtime/collector.test" "$base/runtime/tool-broker" "$proof/frozen-pins.json" > "$proof/binaries.sha256"
 printf 'PASS\n' > "$proof/prepared"
 exit 0
fi
[[ "$mode" == execute ]] || exit 2
test -s "$proof/prepared"
test ! -e "$proof/started"
test "$(jq -r .release "$proof/frozen-pins.json")" = "$release"
auth_value=${4:?private newest encrypted reference}
auth_env=${5:?private matching encryption environment}
[[ "$auth_value" == /srv/storage/butler/agents/*/private/*.value && "$auth_env" == /srv/storage/butler/agents/*/private/*.env ]] || exit 2
test -s "$auth_value"
test -s /srv/storage/butler/agents/local-models/private/token
# Copy opaque private bytes without printing or placing material in arguments.
cp -- "$auth_value" "$base/private/auth-input.value"
sed -n '/^BUTLER_SETTINGS_ENCRYPTION_KEY=/p' "$auth_env" > "$base/private/native.env"
test "$(wc -l < "$base/private/native.env")" = 1
printf '%s\n' 'HTTPS_PROXY=http://127.0.0.1:18080' 'HTTP_PROXY=http://127.0.0.1:18080' 'ALL_PROXY=http://127.0.0.1:18080' 'NO_PROXY=localhost,127.0.0.1,local-models,172.28.0.2,10.237.187.2' >> "$base/private/native.env"
chmod 600 "$base/private/auth-input.value" "$base/private/native.env"
network=butler-issue87-native-v1-net
pg=butler-issue87-native-v1-pg
docker network create --internal --subnet 10.237.187.0/24 "$network" > "$proof/network-id"
mkdir -p "$base/postgres"
docker run --pull never -d --name "$pg" --network "$network" --ip 10.237.187.2 --cpus 1 --memory 512m --pids-limit 128 \
 -v "$base/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_adaptive_test \
 pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB > "$proof/postgres-id"
for i in $(seq 1 60);do docker exec "$pg" pg_isready -U butler -d memory_adaptive_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1;done
date -u +%Y-%m-%dT%H:%M:%SZ > "$proof/started"
collect(){
 docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
 sha256sum "$proof/"*.json "$proof/collector.log" > "$proof/evidence.sha256" || true
}
trap collect EXIT
# Host networking is required by the explicitly pinned local HTTPS proxy.
# The only non-proxy connections are the isolated PG/Broker and pinned local model.
docker run --rm --pull never --network host --user "$(id -u):$(id -g)" --add-host local-models:172.28.0.2 --cpus 2 --memory 2g --pids-limit 128 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$base/runtime:/runtime:ro" -v "$proof:/proof" -v "$base/private:/private" \
 -v /srv/storage/butler/agents/local-models/private/token:/local-token:ro -w /src/apps/orchestrator/internal/app \
 --env-file "$base/private/native.env" -e BUTLER_ADAPTIVE_NATIVE_V1=1 -e BUTLER_ADAPTIVE_NATIVE_PROOF=/proof \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@10.237.187.2:5432/memory_adaptive_test?sslmode=disable' \
 -e BUTLER_MEMORY_READ_APP_BINARY=/runtime/tool-broker -e BUTLER_MEMORY_SEMANTIC_ENABLED=true \
 -e BUTLER_MEMORY_LOCAL_SERVICE_ID=operator-pinned-service -e BUTLER_MEMORY_LOCAL_TOKEN_FILE=/local-token \
 golang:1.27.2-bookworm /runtime/collector.test -test.run '^TestMemoryAdaptiveNativeComparison$' -test.timeout 210m -test.v > "$proof/collector.log" 2>&1
date -u +%Y-%m-%dT%H:%M:%SZ > "$proof/collector-exited"
# The database stays stopped on disk for inspection; no ledger is cleared.
docker stop "$pg" > "$proof/postgres-stopped"

#!/usr/bin/env bash
# Separately sealed observation scope: historical45 + quality236 + paired16.
# No tool routing, retries, fallback, billing assumption or production database.
set -euo pipefail
root=${1:?isolated root required}; phase=${2:-prepare}; release=${3:?exact commit required}
[[ "$root" == /srv/storage/butler/agents/scenario-shadow-high-20261012 && "$phase" =~ ^(prepare|execute|resume-preflight)$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
manifest() { (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | sort -z | xargs -0 sha256sum); }
window=/srv/storage/butler/native-acceptance-windows/issue33-high-shadow-20261012-baseline45
if [[ "$phase" != prepare ]]; then
  if [[ "$phase" == execute ]]; then
    [[ ! -e "$window" && ! -L "$window" ]] || { printf '{"status":"denied","reason":"sealed_window"}\n'; exit 2; }
  else
    # One fixed zero-native permission failure preceded PG, TLS, import and
    # docker create/start. Keep its marker/source/proof; never resume a start.
    failed="$root/reports/shadow-execute-20261012T110125Z-1227245"
    [[ -d "$window" && ! -L "$window" && ! -e "$window/native-started" && ! -e "$window/preflight-continuation" && ! -e "$window/proof-ref" ]]
    [[ "$(cat "$window/release-ref")" == 297287f13b7de91aa48cbfa836001f2216310af1 ]]
    [[ -f "$failed/source.sha256" && -f "$failed/metadata/hardware.json" && ! -e "$failed/pins.txt" && ! -e "$failed/tls.json" && ! -e "$failed/native-events.txt" ]]
    [[ "$(sha256sum "$failed/binaries/orchestrator.test" | cut -d' ' -f1)" == 976e3e53298508850a805b01f69c3a0404bbf0b0f9c531425ac91a4a36725d48 ]]
  fi
  [[ "$(cat "$root/prepared-release")" == "$release" && "$(cat "$root/prepared-source")" == "$source_dir" ]]
  manifest | cmp -s "$root/prepared-manifest" -
  sha256sum -c "$root/prepared-runtime.sha256" >/dev/null
  if [[ "$phase" == execute ]]; then
    mkdir -p /srv/storage/butler/native-acceptance-windows
    mkdir "$window"
    printf '%s\n' "$release" > "$window/release-ref"
    printf '%s\n' "$source_dir" > "$window/source-ref"
  else
    mkdir "$window/preflight-continuation"
    printf '%s\n' "$release" > "$window/preflight-continuation/release-ref"
    printf '%s\n' "$source_dir" > "$window/preflight-continuation/source-ref"
    printf '%s\n' "$failed" > "$window/preflight-continuation/prior-failed-proof-ref"
  fi
fi
stamp=$(date -u +%Y%m%dT%H%M%SZ)-$$
proof="$root/reports/shadow-$phase-$stamp"; name="butler-shadow-$stamp"; network="$name-net"
mkdir -p "$proof/postgres" "$proof/binaries" "$proof/runtime" "$proof/metadata" /srv/storage/butler/.cache/go-tmp
chmod 755 "$proof/binaries"
created_pg=false; created_net=false; created_runtime=false
cleanup() {
  $created_runtime && docker rm -f "$name-runtime" >/dev/null 2>&1 || true
  $created_pg && docker rm -f "$name-pg" >/dev/null 2>&1 || true
  $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
printf 'Private shadow %s proof: %s\n' "$phase" "$proof"
manifest > "$proof/source.sha256"
if [[ "$phase" != prepare ]]; then
  prepared=$(cat "$root/prepared-proof")
  [[ "$prepared" == "$root/reports/shadow-prepare-"* ]]
  cp "$prepared/binaries/"* "$proof/binaries/"
else
  docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$proof/binaries:/binaries" --workdir /src \
    --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/cache/go-tmp \
    golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 ./internal/transport ./internal/transport/antigravity
go test -json -race -p 1 -count=1 -run "^TestCloud" ./internal/acceptance
go test -json -race -p 1 -count=1 ./scripts/dev/delegation-final-tls
go build -o /binaries/tls-prerequisite ./scripts/dev/delegation-final-tls
# runtime.Caller pins /src; do not trimpath the fixture or migrations.
go test -c -race -p 1 -o /binaries/orchestrator.test ./apps/orchestrator/internal/app
go vet ./internal/transport ./internal/transport/antigravity ./internal/acceptance ./apps/orchestrator/internal/app
chmod 555 /binaries/*
' > "$proof/unit.jsonl" 2> "$proof/build-diagnostics.log"
  ! grep -Eq '"Action":"(skip|fail)"' "$proof/unit.jsonl"
fi
review=/srv/storage/butler/reviews/issue33-20261011-r3
cp "$review/public-key.hex" "$proof/metadata/public-key.hex"
cp "$review/approval.json" "$proof/metadata/approval.json"
cp "$review/candidate-files/testdata/acceptance/ru-scenarios-v3.json" "$proof/metadata/frozen-corpus.json"
go_image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
pg_image=$(docker image inspect pgvector/pgvector:pg17 --format '{{.Id}}')
lscpu --json --extended=CPU,CORE,SOCKET,NODE,ONLINE > "$proof/hardware-cpu.json"
cpu_hash=$(sha256sum "$proof/hardware-cpu.json" | cut -d' ' -f1)
printf '{"kernel":"%s","logical_cpus":%s,"host_ram_kib":%s,"cpu_manifest_sha256":"%s","go_image":"%s","postgres_image":"%s","client_ram_bytes":1610612736,"client_cpus":2,"remote_hardware":null,"remote_ram":null,"weights_sha256":null,"incremental_invoice":null}\n' \
  "$(uname -r)" "$(getconf _NPROCESSORS_ONLN)" "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" "$cpu_hash" "$go_image" "$pg_image" > "$proof/metadata/hardware.json"
# Reject hardware/image drift below after chown: both private directories
# belong to10001, and comparison still precedes any authentication.
docker run --rm --network none --user 0:0 --volume "$proof/runtime:/private" --volume "$proof/metadata:/metadata" alpine:3.22 sh -eu -c 'chown -R 10001:10001 /private /metadata; chmod 700 /private /metadata; chmod 600 /metadata/*'
if [[ "$phase" != prepare ]]; then
  docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --security-opt no-new-privileges:true \
    --volume "$proof/metadata:/actual:ro" --volume "$prepared/metadata:/prepared:ro" alpine:3.22 sh -eu -c '
for file in hardware.json public-key.hex approval.json frozen-corpus.json; do cmp "/actual/$file" "/prepared/$file"; done'
fi
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --memory 512m --pids-limit 64 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" --volume "$proof/metadata:/metadata:ro" --workdir /src \
  --env BUTLER_SCENARIO_SHADOW_PREPARE=1 --env BUTLER_SCENARIO_SHADOW_PRIVATE=/private --env "BUTLER_SCENARIO_SHADOW_RELEASE=$release" --env "BUTLER_SCENARIO_SHADOW_RUNTIME_IMAGE=$go_image" --env TMPDIR=/private \
  "$go_image" /binaries/orchestrator.test -test.v -test.timeout=2m -test.run='^TestScenarioShadowPreparePins$' > "$proof/pins.txt" 2>&1
if [[ "$phase" != prepare ]]; then
  docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/actual:ro" --volume "$prepared/runtime:/prepared:ro" alpine:3.22 \
    sh -eu -c 'cmp /actual/pin.json /prepared/pin.json; cmp /actual/policy.json /prepared/policy.json; cmp /actual/resources.json /prepared/resources.json'
fi
docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/private:ro" --volume "$proof/metadata:/metadata" alpine:3.22 sh -eu -c 'cp /private/pin.json /metadata/pin.json; chmod 600 /metadata/pin.json'
docker network create --internal "$network" >/dev/null; created_net=true
docker run -d --name "$name-pg" --runtime runc --network "$network" --network-alias shadow-postgres --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=shadow-fixture --env POSTGRES_DB=butler_shadow_test --volume "$proof/postgres:/var/lib/postgresql/data" "$pg_image" \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null; created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$name-pg" pg_isready -U fixture -d butler_shadow_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1; sleep 1
done
dsn='BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:shadow-fixture@shadow-postgres:5432/butler_shadow_test?sslmode=disable'
if [[ "$phase" == prepare ]]; then
  docker run --rm --runtime runc --network "$network" --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --workdir /src --env "$dsn" \
    "$go_image" /binaries/orchestrator.test -test.v -test.timeout=4m -test.run='^TestScenarioShadow(Window|Sampler|Readiness)' > "$proof/pg-tests.txt" 2>&1
  ! grep -Eq -- '--- (FAIL|SKIP):' "$proof/pg-tests.txt"
  manifest | cmp -s "$proof/source.sha256" -
  sha256sum "$proof/binaries/"* > "$root/prepared-runtime.sha256"
  cp "$proof/source.sha256" "$root/prepared-manifest"
  printf '%s\n' "$proof" > "$root/prepared-proof"
  printf '%s\n' "$source_dir" > "$root/prepared-source"
  printf '%s\n' "$release" > "$root/prepared-release"
  sha256sum "$proof/unit.jsonl" "$proof/pg-tests.txt" "$proof/pins.txt" > "$proof/evidence.sha256"
  printf '{"status":"preparation_passed","native_calls":0,"baseline":45,"maximum_new_attempts":252}\n'; exit 0
fi
printf '%s\n' "$proof" > "$window/proof-ref"
[[ $(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1) == true ]]
docker run --rm --network butler-lab_backend --user 10001:10001 --read-only --cap-drop ALL --memory 64m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
  --volume "$proof/binaries/tls-prerequisite:/prerequisite:ro" "$go_image" /prerequisite > "$proof/tls.json"
import_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
import_file="$import_parent/butler-native-import.json"
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --memory 32m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
  --volume "$import_parent:/metadata:ro" alpine:3.22 sh -eu -c 'p=/metadata/butler-native-import.json; test -f "$p"; test ! -L "$p"; test "$(stat -c "%u:%a" "$p")" = 10001:600; size=$(stat -c %s "$p"); test "$size" -gt 0; test "$size" -le 65536'
docker create --name "$name-runtime" --runtime runc --network "$network" --user 10001:10001 --read-only --cap-drop ALL --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true --workdir /src \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" --volume "$proof/metadata:/metadata:ro" \
  --volume "$import_file:/home/butler/.gemini/antigravity-cli/butler-native-import.json:ro" \
  --env TMPDIR=/private --env HOME=/home/butler --env HTTPS_PROXY=http://antigravity-proxy:18080 --env HTTP_PROXY=http://antigravity-proxy:18080 --env NO_PROXY=localhost,127.0.0.1,shadow-postgres \
  --env "$dsn" --env BUTLER_SCENARIO_SHADOW_ACCEPTANCE=1 --env "BUTLER_SCENARIO_SHADOW_RELEASE=$release" --env BUTLER_SCENARIO_SHADOW_PRIVATE=/private \
  --env BUTLER_SCENARIO_SHADOW_IMPORT_FILE=/home/butler/.gemini/antigravity-cli/butler-native-import.json \
  "$go_image" /binaries/orchestrator.test -test.v -test.timeout=80m -test.run='^TestScenarioShadowActualHighObservationE2E$' >/dev/null; created_runtime=true
docker network connect butler-lab_backend "$name-runtime"
manifest | cmp -s "$root/prepared-manifest" -
status=0
mkdir "$window/native-started"
printf '%s\n' "$proof" > "$window/native-started/proof-ref"
docker start -a "$name-runtime" > "$proof/native-events.txt" 2>&1 || status=$?
# The complete private report and per-attempt immutable files survive all failures.
docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/private:ro" alpine:3.22 cat /private/observations.json > "$proof/observations.json" || status=1
sha256sum "$proof/observations.json" "$proof/native-events.txt" > "$proof/native-evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"shadow_window_passed","maximum_total_attempts":297,"maximum_new_attempts":252,"issue33_ready":false}\n'

#!/usr/bin/env bash
# Separately sealed observation scope: actual historical283 + quality236 + paired16.
# No tool routing, retries, fallback, billing assumption or production database.
set -euo pipefail
root=${1:?isolated root required}; phase=${2:-prepare}; release=${3:?exact commit required}
[[ "$root" == /srv/storage/butler/agents/scenario-shadow-v5-high-20261013 && "$phase" =~ ^(offline|prepare|execute)$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
manifest() { (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum); }
window=/srv/storage/butler/native-acceptance-windows/issue33-high-shadow-v5-20261013-baseline283
if [[ "$phase" == execute ]]; then
  [[ ! -e "$window" && ! -L "$window" ]] || { printf '{"status":"denied","reason":"sealed_window"}\n'; exit 2; }
  [[ "$(cat "$root/prepared-release")" == "$release" && "$(cat "$root/prepared-source")" == "$source_dir" ]]
  manifest | cmp -s "$root/prepared-manifest" -
  sha256sum -c "$root/prepared-runtime.sha256" >/dev/null
  native_deadline=$(date -u -d '+75 minutes' +%Y-%m-%dT%H:%M:%SZ)
fi

stamp=$(date -u +%Y%m%dT%H%M%SZ)-$$
proof="$root/reports/shadow-$phase-$stamp"; name="butler-shadow-$stamp"; network="$name-net"
mkdir -p "$proof/postgres" "$proof/binaries" "$proof/runtime" "$proof/metadata" "$root/.cache/go-tmp"
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
if [[ "$phase" == execute ]]; then
  prepared=$(cat "$root/prepared-proof")
  [[ "$prepared" == "$root/reports/shadow-prepare-"* ]]
  cp "$prepared/binaries/"* "$proof/binaries/"
  chmod 555 "$proof/binaries/"*
else
  docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$root/.cache/go-tmp:/tmp" --volume "$proof/binaries:/binaries" --workdir /src \
    --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/tmp --env TMPDIR=/tmp \
    golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 ./internal/transport ./internal/transport/antigravity
go test -json -race -p 1 -count=1 -run "^TestCloud(V5|FreshV5)" ./internal/acceptance ./cmd/cloud-observe
go test -json -race -p 1 -count=1 ./scripts/dev/delegation-final-tls
go build -o /binaries/tls-prerequisite ./scripts/dev/delegation-final-tls
go test -json -race -p 1 -count=1 ./scripts/dev/cloud-v5-final-audit
go build -o /binaries/final-audit ./scripts/dev/cloud-v5-final-audit
go build -o /binaries/review-seal ./scripts/dev/cloud-v5-review
# runtime.Caller pins /src; do not trimpath the fixture or migrations.
go test -c -race -p 1 -o /binaries/orchestrator.test ./apps/orchestrator/internal/app
go vet ./internal/transport ./internal/transport/antigravity ./internal/acceptance ./cmd/cloud-observe ./apps/orchestrator/internal/app ./scripts/dev/cloud-v5-final-audit ./scripts/dev/cloud-v5-review
chmod 555 /binaries/*
' > "$proof/unit.jsonl" 2> "$proof/build-diagnostics.log"
  ! grep -Eq '"Action":"(skip|fail)"' "$proof/unit.jsonl"
fi
pin_binaries="$proof/binaries"
if [[ "$phase" == prepare ]]; then
  # Exercise the same077-umask copy boundary as actual execution, as10001 below.
  pin_binaries="$proof/copied-binaries"
  mkdir "$pin_binaries"; chmod 755 "$pin_binaries"
  cp "$proof/binaries/"* "$pin_binaries/"
  chmod 555 "$pin_binaries/"*
  for file in orchestrator.test tls-prerequisite final-audit review-seal; do cmp -s "$proof/binaries/$file" "$pin_binaries/$file"; done
fi
review=/srv/storage/butler/reviews/issue33-20261013-v5-corrected-sealed
go_image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
pg_image=$(docker image inspect pgvector/pgvector:pg17 --format '{{.Id}}')
lscpu --json --extended=CPU,CORE,SOCKET,NODE,ONLINE > "$proof/hardware-cpu.json"
cpu_hash=$(sha256sum "$proof/hardware-cpu.json" | cut -d' ' -f1)
printf '{"kernel":"%s","logical_cpus":%s,"host_ram_kib":%s,"cpu_manifest_sha256":"%s","go_image":"%s","postgres_image":"%s","client_ram_bytes":1610612736,"client_cpus":2,"remote_hardware":null,"remote_ram":null,"weights_sha256":null,"incremental_invoice":null}\n' \
  "$(uname -r)" "$(getconf _NPROCESSORS_ONLN)" "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" "$cpu_hash" "$go_image" "$pg_image" > "$proof/metadata/hardware.json"
if [[ "$phase" != offline ]]; then
# Reject hardware/image drift below after chown: both private directories
# belong to10001, and comparison still precedes any authentication.
docker run --rm --network none --user 0:0 --volume "$proof/runtime:/private" --volume "$proof/metadata:/metadata" alpine:3.22 sh -eu -c 'chown -R 10001:10001 /private /metadata; chmod 700 /private /metadata; chmod 600 /metadata/*'
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --volume "$review/public-key.hex:/review/public-key.hex:ro" --volume "$review/approval.json:/review/approval.json:ro" \
  --volume "$review/frozen-corpus.json:/review/frozen-corpus.json:ro" \
  --volume "$proof/metadata:/metadata" alpine:3.22 sh -eu -c '
for file in public-key.hex approval.json frozen-corpus.json; do cp "/review/$file" "/metadata/$file"; chmod 600 "/metadata/$file"; done'
if [[ "$phase" != prepare ]]; then
  docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --security-opt no-new-privileges:true \
    --volume "$proof/metadata:/actual:ro" --volume "$prepared/metadata:/prepared:ro" alpine:3.22 sh -eu -c '
for file in hardware.json public-key.hex approval.json frozen-corpus.json; do cmp "/actual/$file" "/prepared/$file"; done'
fi
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --memory 512m --pids-limit 64 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume "$pin_binaries:/binaries:ro" --volume "$proof/runtime:/private" --volume "$proof/metadata:/metadata:ro" --workdir /src \
  --env BUTLER_SCENARIO_SHADOW_V5_PREPARE=1 --env BUTLER_SCENARIO_SHADOW_V5_PRIVATE=/private --env "BUTLER_SCENARIO_SHADOW_V5_RELEASE=$release" --env "BUTLER_SCENARIO_SHADOW_V5_RUNTIME_IMAGE=$go_image" --env TMPDIR=/private \
  "$go_image" /binaries/orchestrator.test -test.v -test.timeout=2m -test.run='^TestScenarioShadowV5PreparePins$' > "$proof/pins.txt" 2>&1
if [[ "$phase" != prepare ]]; then
  docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/actual:ro" --volume "$prepared/runtime:/prepared:ro" alpine:3.22 \
    sh -eu -c 'for file in pin.json policy.json resources.json history.json compatibility.json; do cmp "/actual/$file" "/prepared/$file"; done'
fi
docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/private:ro" --volume "$proof/metadata:/metadata" alpine:3.22 sh -eu -c 'for file in pin.json history.json compatibility.json; do cp "/private/$file" "/metadata/$file"; chmod 600 "/metadata/$file"; done'
fi
docker network create --internal "$network" >/dev/null; created_net=true
docker run -d --name "$name-pg" --runtime runc --network "$network" --network-alias shadow-postgres --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=shadow-fixture --env POSTGRES_DB=butler_shadow_v5_test --volume "$proof/postgres:/var/lib/postgresql/data" "$pg_image" \
    -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null; created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$name-pg" pg_isready -U fixture -d butler_shadow_v5_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1; sleep 1
done
dsn='BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:shadow-fixture@shadow-postgres:5432/butler_shadow_v5_test?sslmode=disable'
if [[ "$phase" == prepare || "$phase" == offline ]]; then
  docker run --rm --runtime runc --network "$network" --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --workdir /src --env "$dsn" \
    "$go_image" /binaries/orchestrator.test -test.v -test.timeout=10m -test.run='^TestScenario(ShadowV5(Window|Sampler|Readiness|Diagnostic|Synthesis|PrivateFinal|NativeSchemas)|HighV5(Quality)?History|PrivateFinalCapture)' > "$proof/pg-tests.txt" 2>&1
  ! grep -Eq -- '--- (FAIL|SKIP):' "$proof/pg-tests.txt"
  LC_ALL=C sort "$source_dir/scripts/dev/fixtures/scenario-shadow-v5-required-tests.txt" > "$proof/required-tests.txt"
  sed -n 's/^[[:space:]]*--- PASS: \(TestScenarioShadowV5[^ ]*\).*/\1/p' "$proof/pg-tests.txt" | LC_ALL=C sort > "$proof/passed-tests.txt"
  cmp "$proof/required-tests.txt" "$proof/passed-tests.txt"
  # A fresh prepare-only SQL fixture checks the exact independent exporter;
  # execute always creates a different empty PostgreSQL volume/container.
  docker exec -i "$name-pg" psql -q -v ON_ERROR_STOP=1 -U fixture -d butler_shadow_v5_test < "$source_dir/scripts/dev/fixtures/scenario-shadow-v5-pg-probe.sql"
  docker exec -i "$name-pg" psql -q -A -t -v ON_ERROR_STOP=1 -U fixture -d butler_shadow_v5_test < "$source_dir/scripts/dev/fixtures/scenario-shadow-v5-pg-witness.sql" > "$proof/pg-probe.json"
  docker run --rm --runtime runc --network none --read-only --cap-drop ALL --user "$(id -u):$(id -g)" --security-opt no-new-privileges:true --memory 512m --pids-limit 64 \
    --volume "$proof/pg-probe.json:/pg-witness.json:ro" --volume "$proof/binaries:/binaries:ro" \
    --env BUTLER_SCENARIO_SHADOW_V5_SQL_PROBE=1 "$go_image" /binaries/orchestrator.test -test.v -test.timeout=30s -test.run='^TestScenarioShadowV5SQLExportProbe$' > "$proof/pg-probe-test.txt" 2>&1
  [[ $(grep -c '^--- PASS: TestScenarioShadowV5SQLExportProbe ' "$proof/pg-probe-test.txt") == 1 ]]
  ! grep -Eq -- '--- (FAIL|SKIP):' "$proof/pg-probe-test.txt"
  manifest | cmp -s "$proof/source.sha256" -
  if [[ "$phase" == offline ]]; then
    sha256sum "$proof/unit.jsonl" "$proof/pg-tests.txt" "$proof/pg-probe.json" "$proof/pg-probe-test.txt" "$proof/source.sha256" > "$proof/evidence.sha256"
    printf '{"status":"offline_unreviewed_candidate_passed","native_calls":0,"baseline":283,"maximum_new_attempts":252,"execution_enabled":false}\n' > "$proof/summary.json"
    printf 'PASS: V5 candidate offline guards; no signed preparation, auth, native marker or model invocation.\n'
    exit 0
  fi
  sha256sum "$proof/binaries/"* > "$root/prepared-runtime.sha256"
  cp "$proof/source.sha256" "$root/prepared-manifest"
  printf '%s\n' "$proof" > "$root/prepared-proof"
  printf '%s\n' "$source_dir" > "$root/prepared-source"
  printf '%s\n' "$release" > "$root/prepared-release"
  sha256sum "$proof/unit.jsonl" "$proof/pg-tests.txt" "$proof/pins.txt" "$proof/pg-probe.json" "$proof/pg-probe-test.txt" > "$proof/evidence.sha256"
  printf '{"status":"preparation_passed","native_calls":0,"baseline":283,"maximum_new_attempts":252}\n'; exit 0
fi
[[ $(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1) == true ]]
docker run --rm --network butler-lab_backend --user 10001:10001 --read-only --cap-drop ALL --memory 64m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
  --volume "$proof/binaries/tls-prerequisite:/prerequisite:ro" "$go_image" /prerequisite > "$proof/tls.json"
import_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
import_file="$import_parent/butler-native-import.json"
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --memory 32m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
  --volume "$import_parent:/metadata:ro" alpine:3.22 sh -eu -c 'p=/metadata/butler-native-import.json; test -f "$p"; test ! -L "$p"; test "$(stat -c "%u:%a" "$p")" = 10001:600; size=$(stat -c %s "$p"); test "$size" -gt 0; test "$size" -le 65536'
manifest | cmp -s "$root/prepared-manifest" -
mkdir -p /srv/storage/butler/native-acceptance-windows
mkdir "$window"
printf '%s\n' "$release" > "$window/release-ref"
printf '%s\n' "$source_dir" > "$window/source-ref"
printf '%s\n' "$proof" > "$window/proof-ref"
docker create --name "$name-runtime" --runtime runc --network "$network" --user 10001:10001 --read-only --cap-drop ALL --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true --workdir /src \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" --volume "$proof/metadata:/metadata:ro" \
  --volume "$import_file:/home/butler/.gemini/antigravity-cli/butler-native-import.json:ro" \
  --env TMPDIR=/private --env HOME=/home/butler --env HTTPS_PROXY=http://antigravity-proxy:18080 --env HTTP_PROXY=http://antigravity-proxy:18080 --env NO_PROXY=localhost,127.0.0.1,shadow-postgres \
  --env "$dsn" --env BUTLER_SCENARIO_SHADOW_V5_ACCEPTANCE=1 --env "BUTLER_SCENARIO_SHADOW_V5_RELEASE=$release" --env BUTLER_SCENARIO_SHADOW_V5_PRIVATE=/private \
  --env "BUTLER_SCENARIO_SHADOW_V5_DEADLINE=$native_deadline" --env "BUTLER_SCENARIO_SHADOW_V5_RUNTIME_IMAGE=$go_image" \
  --env BUTLER_SCENARIO_SHADOW_V5_IMPORT_FILE=/home/butler/.gemini/antigravity-cli/butler-native-import.json \
  "$go_image" /binaries/orchestrator.test -test.v -test.timeout=80m -test.run='^TestScenarioShadowV5ActualHighObservationE2E$' >/dev/null; created_runtime=true
docker network connect butler-lab_backend "$name-runtime"
manifest | cmp -s "$root/prepared-manifest" -
status=0
mkdir "$window/native-started"
printf '%s\n' "$proof" > "$window/native-started/proof-ref"
docker start -a "$name-runtime" > "$proof/native-events.txt" 2>&1 || status=$?
# A separate read-only PostgreSQL snapshot is independent of collector report
# serialization. Select only these immutable fixture metadata/usage columns.
docker exec -i "$name-pg" psql -q -A -t -v ON_ERROR_STOP=1 -U fixture -d butler_shadow_v5_test < "$source_dir/scripts/dev/fixtures/scenario-shadow-v5-pg-witness.sql" > "$proof/pg-witness.json" 2> "$proof/pg-witness-diagnostics.log" || status=1
docker exec -i "$name-pg" psql -q -A -t -v ON_ERROR_STOP=1 -U fixture -d butler_shadow_v5_test < "$source_dir/scripts/dev/fixtures/scenario-shadow-v5-pg-diagnostics.sql" > "$proof/pg-diagnostics.json" 2> "$proof/pg-diagnostics-error.log" || status=1
# The complete private report and per-attempt immutable files survive all failures.
docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/private:ro" alpine:3.22 cat /private/observations.json > "$proof/observations.json" || status=1
sha256sum "$proof/observations.json" "$proof/native-events.txt" "$proof/pg-witness.json" "$proof/pg-diagnostics.json" > "$proof/native-evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"shadow_window_passed","maximum_total_attempts":535,"maximum_new_attempts":252,"issue33_ready":false}\n'

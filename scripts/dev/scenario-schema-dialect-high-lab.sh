#!/usr/bin/env bash
# Separate prospective format diagnostic; no benchmark retry or quality verdict.
set -euo pipefail
root=${1:?isolated root required}; phase=${2:-prepare}; release=${3:?exact source commit required}
[[ "$root" == /srv/storage/butler/agents/scenario-schema-dialect-high-20261013 && "$phase" =~ ^(prepare|execute)$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
manifest() { (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum); }
window=/srv/storage/butler/native-acceptance-windows/issue33-high-schema-dialect-20261013-baseline279
if [[ "$phase" == execute ]]; then
  [[ ! -e "$window" && ! -L "$window" ]] || exit 2
  [[ "$(cat "$root/prepared-release")" == "$release" && "$(cat "$root/prepared-source")" == "$source_dir" ]]
  manifest | cmp -s "$root/prepared-manifest" -
  sha256sum -c "$root/prepared-runtime.sha256" >/dev/null
fi
stamp=$(date -u +%Y%m%dT%H%M%SZ)-$$
proof="$root/reports/schema-dialect-$phase-$stamp"; name="butler-schema-dialect-$stamp"; network="$name-net"
mkdir -p "$proof/postgres" "$proof/binaries" "$proof/runtime" "$proof/tmp" "$root/.cache/go-tmp"
chmod 755 "$proof/binaries"
created_pg=false; created_net=false; created_runtime=false
cleanup() {
  $created_runtime && docker rm -f "$name-runtime" >/dev/null 2>&1 || true
  $created_pg && docker rm -f "$name-pg" >/dev/null 2>&1 || true
  $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
printf 'Private schema-dialect diagnostic %s proof: %s\n' "$phase" "$proof"
manifest > "$proof/source.sha256"
printf '%s\n' "$release" > "$proof/source-commit.txt"
go_image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
pg_image=$(docker image inspect pgvector/pgvector:pg17 --format '{{.Id}}')
lscpu --json --extended=CPU,CORE,SOCKET,NODE,ONLINE > "$proof/hardware-cpu.json"
cpu_hash=$(sha256sum "$proof/hardware-cpu.json" | cut -d' ' -f1)
printf '{"kernel":"%s","logical_cpus":%s,"host_ram_kib":%s,"cpu_manifest_sha256":"%s","go_image":"%s","postgres_image":"%s","client_ram_bytes":1610612736,"client_cpus":2,"cloud_ram":null,"weights_sha256":null,"invoice":null}\n' \
  "$(uname -r)" "$(getconf _NPROCESSORS_ONLN)" "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" "$cpu_hash" "$go_image" "$pg_image" > "$proof/hardware.json"
if [[ "$phase" == prepare ]]; then
  docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 160 --cpus 2 --cap-drop ALL --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$root/.cache/go-tmp:/tmp" --volume "$proof/binaries:/binaries" --workdir /src \
    --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/tmp --env TMPDIR=/tmp \
    "$go_image" bash -eu -c '
go test -json -race -p 1 -count=1 -run "^TestCloud(V5|FreshV5|SchemaDialectDiagnostic)" ./internal/acceptance
go test -json -race -p 1 -count=1 ./internal/transport/antigravity ./internal/transport
go test -json -race -p 1 -count=1 -run "^TestSchemaDialectFactoryUnknownSelection" ./apps/orchestrator/internal/app
go test -json -race -p 1 -count=1 ./scripts/dev/delegation-final-tls
go build -o /binaries/tls-prerequisite ./scripts/dev/delegation-final-tls
go test -c -race -p 1 -o /binaries/orchestrator.test ./apps/orchestrator/internal/app
go vet ./apps/orchestrator/internal/app ./internal/acceptance ./internal/transport/antigravity ./internal/transport ./scripts/dev/delegation-final-tls
chmod 555 /binaries/*
' > "$proof/unit.jsonl" 2> "$proof/build-diagnostics.log"
  ! grep -Eq '"Action":"(skip|fail)"' "$proof/unit.jsonl"
else
  prepared=$(cat "$root/prepared-proof")
  [[ "$prepared" == "$root/reports/schema-dialect-prepare-"* ]]
  cmp "$prepared/hardware.json" "$proof/hardware.json"
  cp "$prepared/binaries/"* "$proof/binaries/"; chmod 555 "$proof/binaries/"*
  for file in orchestrator.test tls-prerequisite; do cmp "$prepared/binaries/$file" "$proof/binaries/$file"; done
  sha256sum "$proof/binaries/"* > "$proof/runtime.sha256"
  # This independent source gate precedes TLS, import inspection and markers.
  docker run --rm --runtime runc --network none --read-only --user 10001:10001 --cap-drop ALL --memory 512m --pids-limit 64 --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --workdir /src --env BUTLER_SCHEMA_DIALECT_GATE=1 \
    "$go_image" /binaries/orchestrator.test -test.v -test.timeout=30s -test.run='^TestScenarioSchemaDialectOperationalGate$' > "$proof/gate.txt" 2>&1
  [[ $(grep -c '^--- PASS: TestScenarioSchemaDialectOperationalGate ' "$proof/gate.txt") == 1 ]]
fi
docker network create --internal "$network" >/dev/null; created_net=true
docker run -d --name "$name-pg" --runtime runc --network "$network" --network-alias schema-dialect-postgres --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=schema-dialect-fixture --env POSTGRES_DB=butler_schema_dialect_test --volume "$proof/postgres:/var/lib/postgresql/data" "$pg_image" \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null; created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$name-pg" pg_isready -U fixture -d butler_schema_dialect_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1; sleep 1
done
dsn='BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:schema-dialect-fixture@schema-dialect-postgres:5432/butler_schema_dialect_test?sslmode=disable'
if [[ "$phase" == prepare ]]; then
  docker run --rm --runtime runc --network "$network" --read-only --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 160 --cpus 2 --cap-drop ALL --security-opt no-new-privileges:true \
    --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/tmp:/tmp" --workdir /src --env TMPDIR=/tmp --env "$dsn" \
    "$go_image" /binaries/orchestrator.test -test.v -test.timeout=5m \
      -test.run='^TestScenario(PrivateFinalCapture|HighV5History|SchemaDialect(ClosedGateAndExactInputs|EncodingOnlyFourContextsIntegration|FailuresDenyNextEntryIntegration|PostWaitGuardsIntegration|ChangedInputDeniedBeforeEntryIntegration|PendingReconstructionDeniesIntegration|AbsoluteDeadlinePersistedIntegration|WholeEntryDeadlineIntegration|OrderAndUnknownModeDenyBeforeEntryIntegration|ReadOnlyWitnessProbeIntegration|AuditOriginalSQLAndTamperDenialsIntegration))' > "$proof/pg-tests.txt" 2>&1
  ! grep -Eq -- '--- (FAIL|SKIP):' "$proof/pg-tests.txt"
  LC_ALL=C sort "$source_dir/scripts/dev/fixtures/schema-dialect-required-tests.txt" > "$proof/required-tests.txt"
  sed -n 's/^[[:space:]]*--- PASS: \(TestScenarioSchemaDialect[^ ]*\).*/\1/p' "$proof/pg-tests.txt" | LC_ALL=C sort > "$proof/passed-tests.txt"
  cmp "$proof/required-tests.txt" "$proof/passed-tests.txt"
  manifest | cmp -s "$proof/source.sha256" -
  sha256sum "$proof/binaries/"* > "$proof/runtime.sha256"
  sha256sum "$proof/unit.jsonl" "$proof/pg-tests.txt" "$proof/source.sha256" "$proof/source-commit.txt" "$proof/runtime.sha256" "$proof/hardware.json" > "$proof/evidence.sha256"
  sha256sum "$proof/binaries/"* > "$root/prepared-runtime.sha256"
  cp "$proof/source.sha256" "$root/prepared-manifest"
  printf '%s\n' "$proof" > "$root/prepared-proof"
  printf '%s\n' "$source_dir" > "$root/prepared-source"
  printf '%s\n' "$release" > "$root/prepared-release"
  printf '{"status":"closed_diagnostic_offline_guards_passed","native_calls":0,"baseline":279,"ceiling":283,"maximum_new":4,"issue33_ready":false,"quality_claim":false}\n' > "$proof/summary.json"
  exit 0
fi
[[ $(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1) == true ]]
docker run --rm --runtime runc --network butler-lab_backend --user 10001:10001 --read-only --cap-drop ALL --memory 64m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
  --volume "$proof/binaries/tls-prerequisite:/prerequisite:ro" "$go_image" /prerequisite > "$proof/tls.json"
import_file=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli/butler-native-import.json
# SSH operator lacks search permission in the protected10001 parent. A separate
# fixed metadata-only checker sees the original leaf, not Docker's resolved bind.
# It has no model/provider/auth code and neither reads nor lists file contents.
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --memory 32m --pids-limit 16 --security-opt no-new-privileges:true \
  --volume /srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli:/metadata:ro alpine:3.22 sh -eu -c 'p=/metadata/butler-native-import.json; test -f "$p"; test ! -L "$p"; test "$(stat -c "%u:%a" "$p")" = 10001:600; size=$(stat -c %s "$p"); test "$size" -gt 0; test "$size" -le 65536'
docker run --rm --network none --user 0:0 --volume "$proof/runtime:/private" --volume "$proof/tmp:/tmp" alpine:3.22 sh -eu -c 'chown 10001:10001 /private /tmp; chmod 700 /private /tmp'
manifest | cmp -s "$root/prepared-manifest" -
mkdir -p /srv/storage/butler/native-acceptance-windows
mkdir "$window"
deadline=$(date -u -d '+600 seconds' +%Y-%m-%dT%H:%M:%SZ)
printf '%s\n' "$release" > "$window/release-ref"
printf '%s\n' "$proof" > "$window/proof-ref"
printf '%s\n' "$deadline" > "$window/deadline"
docker create --name "$name-runtime" --runtime runc --network "$network" --user 10001:10001 --read-only --cap-drop ALL --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true --workdir /src \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" --volume "$proof/tmp:/tmp" \
  --volume "$import_file:/home/butler/.gemini/antigravity-cli/butler-native-import.json:ro" \
  --env TMPDIR=/tmp --env HOME=/home/butler --env HTTPS_PROXY=http://antigravity-proxy:18080 --env HTTP_PROXY=http://antigravity-proxy:18080 --env NO_PROXY=localhost,127.0.0.1,schema-dialect-postgres \
  --env "$dsn" --env BUTLER_SCHEMA_DIALECT_ACCEPTANCE=1 --env BUTLER_SCHEMA_DIALECT_PRIVATE=/private --env "BUTLER_SCHEMA_DIALECT_DEADLINE=$deadline" \
  --env BUTLER_SCHEMA_DIALECT_IMPORT_FILE=/home/butler/.gemini/antigravity-cli/butler-native-import.json \
  "$go_image" /binaries/orchestrator.test -test.v -test.timeout=11m -test.run='^TestScenarioSchemaDialectActualHighE2E$' >/dev/null; created_runtime=true
docker network connect butler-lab_backend "$name-runtime"
manifest | cmp -s "$root/prepared-manifest" -
sha256sum -c "$proof/runtime.sha256" >/dev/null
mkdir "$window/native-started" # Atomic once-only entry; never reset or resume.
status=0
docker start -a "$name-runtime" > "$proof/native-events.txt" 2>&1 || status=$?
docker exec -i "$name-pg" psql -q -A -t -v ON_ERROR_STOP=1 -U fixture -d butler_schema_dialect_test < "$source_dir/scripts/dev/fixtures/schema-dialect-pg-witness.sql" > "$proof/pg-witness.json" 2> "$proof/pg-witness-diagnostics.log" || status=1
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL --volume "$proof/runtime:/private:ro" alpine:3.22 cat /private/diagnostic.json > "$proof/diagnostic.json" || status=1
# Raw final-280..283.raw stays in the private0700 runtime directory, never copied.
sha256sum "$proof/native-events.txt" "$proof/diagnostic.json" "$proof/pg-witness.json" > "$proof/native-evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"schema_dialect_diagnostic_completed","issue33_ready":false,"quality_claim":false,"maximum_new":4}\n'

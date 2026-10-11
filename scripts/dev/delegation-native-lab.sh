#!/usr/bin/env bash
# Isolated native experiment. No deployment, shared DB mutation or proxy restart.
set -euo pipefail
root=${1:?isolated agent root required}
phase=${2:-prepare}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$phase" == prepare || "$phase" == execute ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/app/delegation_native_scenario_test.go" ]] || exit 2
export DOCKER_CONFIG="$root/.cache/docker"
if [[ "$phase" == execute ]]; then
  if [[ ${BUTLER_DELEGATION_NATIVE_ACCEPTANCE:-} != 1 ]]; then
    printf '{"status":"skipped","reason":"native_opt_in_unset","new_native_attempts":0,"historical_native_attempts":5,"total_native_attempts":5}\n'
    exit 0
  fi
  [[ ${BUTLER_DELEGATION_NATIVE_RELEASE:-} =~ ^[a-f0-9]{40}$ ]] || exit 2
  # Opaque operator-confirmed reference; never source/read/print credential data.
  import_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
  import_file="$import_parent/butler-native-import.json"
  # The host shell deliberately cannot traverse the private UID10001 parent.
  # This exact literal parent is mounted only for metadata, without reading the
  # import content. The actual inference runtime below mounts only the file.
  docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL \
    --memory 32m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
    --volume "$import_parent:/metadata:ro" alpine:3.22 sh -eu -c '
approved=/metadata/butler-native-import.json
test -f "$approved"
test ! -L "$approved"
test "$(stat -c "%u:%a" "$approved")" = 10001:600
size=$(stat -c %s "$approved")
test "$size" -gt 0
test "$size" -le 65536' || exit 2
  [[ $(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1) == true ]] || exit 2
fi
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/delegation-native-$phase-$run_id"
name="butler-delegation-native-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$proof/binaries" "$proof/runtime" "$root/.cache/delegation-native-tmp"
chmod 755 "$proof/binaries"
created_network=false
created_pg=false
created_runtime=false
cleanup() {
  $created_runtime && docker rm -f "$name-runtime" >/dev/null 2>&1 || true
  $created_pg && docker logs --tail 40 "$name" > "$proof/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$name" --network "$network" --network-alias delegation-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=delegation-fixture --env POSTGRES_DB=butler_delegation_native_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$name" pg_isready -U fixture -d butler_delegation_native_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private native %s evidence: %s\n' "$phase" "$proof"
sha256sum "$source_dir"/apps/orchestrator/internal/app/delegation_native*_test.go \
  "$source_dir/apps/tool-broker/internal/broker/delegation_native_harness_test.go" \
  "$source_dir/scripts/dev/fixtures/delegation-native-research.json" \
  "$source_dir/scripts/dev/delegation-native-lab.sh" > "$proof/source.sha256"
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/delegation-native-tmp:/private-tmp" --volume "$proof/binaries:/binaries" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_native_test?sslmode=disable' \
  --env BUTLER_DELEGATION_NATIVE_CORPUS=/src/scripts/dev/fixtures/delegation-native-research.json \
  golang:1.27.2-bookworm bash -eu -c 'go test -race -c -o /binaries/orchestrator.test ./apps/orchestrator/internal/app
go test -race -c -o /binaries/broker.test ./apps/tool-broker/internal/broker
chmod 555 /binaries/orchestrator.test /binaries/broker.test
export BUTLER_DELEGATION_BROKER_BINARY=/binaries/broker.test
go test -json -race -run "^TestNativeDelegation(ObjectiveFixtureRejectsNarrativeAuthority|TwelveAttemptBoundIntegration|CarryoverBaselineRejectsMismatch|EveryReservationRevalidatesBothArtifactsIntegration|ProviderMethodsAreChargedBeforeValidationIntegration|PrivateRuntimeFiles|EncryptedFixtureImportWithoutInferenceIntegration|CompositionWithoutInferenceIntegration|BrokerPreflightWithoutInferenceIntegration)$" -count=1 -timeout=90s ./apps/orchestrator/internal/app' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
docker run --rm --network none --user 0:0 --volume "$proof/runtime:/private" alpine:3.22 \
  sh -eu -c 'chown 10001:10001 /private; chmod 700 /private'
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL \
  --memory 512m --pids-limit 64 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" --env TMPDIR=/private \
  golang:1.27.2-bookworm /binaries/orchestrator.test -test.v -test.run='^TestNativeDelegation(PrivateRuntimeFiles|CarryoverBaselineRejectsMismatch)$' \
  > "$proof/runtime-files.txt"
sha256sum "$proof/runtime-files.txt" > "$proof/runtime-files.sha256"
if [[ "$phase" == prepare ]]; then
  printf '{"status":"preparation_passed","new_native_attempts":0,"historical_native_attempts":5,"total_native_attempts":5,"maximum_total_native_attempts":17,"maximum_new_native_attempts":12,"model_acceptance":"pending"}\n'
  exit 0
fi
# The preparation probe already verified executable/private-file access as the
# exact runtime UID. Only the execute phase mounts the opaque auth file.
docker create --name "$name-runtime" --user 10001:10001 --network "$network" \
  --memory 1536m --pids-limit 128 --cpus 2 --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" \
  --volume "$import_file:/home/butler/.gemini/antigravity-cli/butler-native-import.json:ro" \
  --env HOME=/home/butler --env TMPDIR=/private \
  --env HTTPS_PROXY=http://antigravity-proxy:18080 --env HTTP_PROXY=http://antigravity-proxy:18080 \
  --env NO_PROXY=localhost,127.0.0.1,delegation-postgres \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_native_test?sslmode=disable' \
  --env BUTLER_DELEGATION_NATIVE_ACCEPTANCE=1 --env "BUTLER_DELEGATION_NATIVE_RELEASE=$BUTLER_DELEGATION_NATIVE_RELEASE" \
  --env BUTLER_DELEGATION_NATIVE_CORPUS=/src/scripts/dev/fixtures/delegation-native-research.json \
  --env BUTLER_DELEGATION_NATIVE_IMPORT_FILE=/home/butler/.gemini/antigravity-cli/butler-native-import.json \
  --env BUTLER_DELEGATION_NATIVE_WORKER_BINARY=/binaries/orchestrator.test --env BUTLER_DELEGATION_BROKER_BINARY=/binaries/broker.test \
  --env BUTLER_DELEGATION_NATIVE_REPORT=/private/scenario.json \
  golang:1.27.2-bookworm /binaries/orchestrator.test -test.v -test.run='^TestNativeDelegationActualScenarioIntegration$' -test.timeout=19m >/dev/null
created_runtime=true
docker network connect butler-lab_backend "$name-runtime"
# A single invocation. Nonzero status is retained, never retried.
status=0
docker start -a "$name-runtime" > "$proof/native-events.txt" 2>&1 || status=$?
docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/private:ro" alpine:3.22 \
  cat /private/scenario.json > "$proof/scenario.json" || status=1
sha256sum "$proof/scenario.json" "$proof/native-events.txt" > "$proof/native-evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"experiment_passed","historical_native_attempts":5,"maximum_total_native_attempts":17,"maximum_new_native_attempts":12,"issue_closure":"pending_audit"}\n'

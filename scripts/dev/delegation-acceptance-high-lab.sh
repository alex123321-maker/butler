#!/usr/bin/env bash
# New High window carrying all 27 attempts; historical runners remain sealed.
set -euo pipefail
root=${1:?isolated agent root required}
phase=${2:-prepare}
[[ "$root" == /srv/storage/butler/agents/delegation-acceptance-high-20261012 ]] || exit 2
[[ "$phase" == prepare || "$phase" == execute ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/app/delegation_native_scenario_test.go" ]] || exit 2
[[ ${BUTLER_DELEGATION_NATIVE_RELEASE:-} =~ ^[a-f0-9]{40}$ ]] || exit 2
source_manifest() {
  (cd "$source_dir"
   [[ -z "$(find . ! -type f ! -type d -print -quit)" ]] || exit 2
   find . -type f -print0 | sort -z | xargs -0 sha256sum)
}
check_tested_source() {
  source_manifest | cmp -s "$root/tested-full-high-manifest" -
}
window=/srv/storage/butler/native-acceptance-windows/high-delegation-acceptance-20261012-baseline27
if [[ "$phase" == execute && ( -e "$source_dir/deploy/delegation/native-acceptance-high-20261012.json" || -L "$source_dir/deploy/delegation/native-acceptance-high-20261012.json" || -e "$window" || -L "$window" ) ]]; then
  # A distinct global marker prevents resetting this new window. Earlier
  # baseline13 and High text-access markers are never reopened or removed.
  printf '{"status":"denied","reason":"native_window_already_attempted","new_native_attempts":0,"fixture_baseline_native_attempts":27}\n'
  exit 2
fi
export DOCKER_CONFIG="$root/.cache/docker"
if [[ "$phase" == execute ]]; then
  if [[ ${BUTLER_DELEGATION_NATIVE_ACCEPTANCE:-} != 1 ]]; then
    printf '{"status":"skipped","reason":"native_opt_in_unset","new_native_attempts":0,"historical_native_attempts":27,"total_native_attempts":27}\n'
    exit 0
  fi
  [[ ${BUTLER_DELEGATION_NATIVE_RELEASE:-} =~ ^[a-f0-9]{40}$ ]] || exit 2
  [[ "$(cat "$root/tested-full-high-source")" == "$source_dir" ]] || exit 2
  [[ "$(cat "$root/tested-full-high-release")" == "$BUTLER_DELEGATION_NATIVE_RELEASE" ]] || exit 2
  check_tested_source || exit 2
  # Seal after exact-source admission and before any auth metadata or Docker.
  # A later preparation/execution failure cannot reopen this invocation.
  mkdir -p /srv/storage/butler/native-acceptance-windows
  mkdir "$window" || exit 2
  printf '%s\n' "$source_dir" > "$window/source-ref"
  printf '%s\n' "$BUTLER_DELEGATION_NATIVE_RELEASE" > "$window/release-ref"
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
proof="$root/reports/delegation-acceptance-high-$phase-$run_id"
name="butler-delegation-acceptance-high-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$proof/binaries" "$proof/runtime" "$root/.cache/delegation-native-tmp"
chmod 755 "$proof/binaries"
created_network=false
created_pg=false
created_runtime=false
cleanup() {
  $created_runtime && docker rm -f "$name-runtime" >/dev/null 2>&1 || true
  $created_pg && docker exec "$name" pg_dump -U fixture -d butler_delegation_native_test > "$proof/ledger.sql" 2>/dev/null || true
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
# Pin the complete snapshot, including the new limiter/runner and all runtime
# dependencies. A path string alone cannot attest unchanged tested bytes.
source_manifest > "$proof/source.sha256"
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/delegation-native-tmp:/private-tmp" --volume "$proof/binaries:/binaries" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_native_test?sslmode=disable' \
  --env 'BUTLER_TEST_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_native_test?sslmode=disable' \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_native_test?sslmode=disable' \
  --env 'BUTLER_TEST_INITIATIVE_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_native_test?sslmode=disable' \
  --env BUTLER_DELEGATION_NATIVE_CORPUS=/src/scripts/dev/fixtures/delegation-high-conflict-20261012.json \
  golang:1.27.2-bookworm bash -eu -c 'go test -race -c -o /binaries/orchestrator.test ./apps/orchestrator/internal/app
go test -race -c -o /binaries/broker.test ./apps/tool-broker/internal/broker
chmod 555 /binaries/orchestrator.test /binaries/broker.test
export BUTLER_DELEGATION_BROKER_BINARY=/binaries/broker.test
go test -json -race -run "^Test(FullHighDelegation(History|LimitIntegration|RevalidationIntegration|CompositionIntegration|IndependentDecisionsWithoutInferenceIntegration|ExpiryAfterUnchangedRowWaitIntegration)|NativeReceiptProjection|Delegation|NativeDelegation(PrivateRuntimeFiles|EncryptedFixtureImportWithoutInferenceIntegration|BrokerPreflightWithoutInferenceIntegration))" -count=1 -timeout=4m ./apps/orchestrator/internal/app
go test -json -race -count=1 -timeout=90s ./internal/delegation
go test -json -race -count=1 -timeout=90s -run "^Test(Delegation|ChildAdmissionCannotResetRootBudgetIntegration|OwnerTreeStopRevokesActualGrandchildGrantIntegration|OwnerStopAndAtomicResultCommitOrderIntegration|ControlStopPropagatesChildrenAndRetainsExternalObservations)" ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api
go test -json -race -count=1 -timeout=90s -run "^Test(DelegationNumeric|ExecuteFinalBelongsToLastModelTurn|RecoveryDelegationReceiptsKeepExactIDsAsData)" ./apps/orchestrator/internal/orchestrator
go test -json -race -count=1 -timeout=90s -run "^TestFlashHigh(SequentialAdmissionReceiptsKeepExactEnvelopeAndFinal|CloudCodeToolContinuation)" ./internal/transport/antigravity
go test -json -race -count=1 -timeout=90s -run "^Test(SharedProvider|ProcessWarmReuseRequiresSameRevisionAndProfile)" ./internal/transport/ollama ./internal/localmodels' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
if grep -q '"Action":"skip"' "$proof/tests.jsonl"; then
  printf '{"status":"preparation_failed","reason":"unexpected_skipped_check","new_native_attempts":0}\n'
  exit 1
fi
docker run --rm --network none --user 0:0 --volume "$proof/runtime:/private" alpine:3.22 \
  sh -eu -c 'chown 10001:10001 /private; chmod 700 /private'
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL \
  --memory 512m --pids-limit 64 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" --env TMPDIR=/private \
  golang:1.27.2-bookworm /binaries/orchestrator.test -test.v -test.run='^TestNativeDelegation(PrivateRuntimeFiles|CarryoverBaselineRejectsMismatch)$' \
  > "$proof/runtime-files.txt"
sha256sum "$proof/runtime-files.txt" > "$proof/runtime-files.sha256"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 128 --cpus 2 \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
 golang:1.27.2-bookworm go vet ./apps/orchestrator/internal/app ./apps/tool-broker/internal/broker ./apps/orchestrator/internal/task ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./internal/delegation
if [[ "$phase" == prepare ]]; then
  source_manifest | cmp -s "$proof/source.sha256" - || exit 2
  cp "$proof/source.sha256" "$root/tested-full-high-manifest"
  printf '%s\n' "$BUTLER_DELEGATION_NATIVE_RELEASE" > "$root/tested-full-high-release"
  printf '%s\n' "$source_dir" > "$root/tested-full-high-source"
  printf '{"status":"preparation_passed","new_native_attempts":0,"fixture_baseline_native_attempts":27,"maximum_total_native_attempts":39,"maximum_new_native_attempts":12,"model_acceptance":"pending"}\n'
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
  --env BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW=1 --env BUTLER_DELEGATION_NATIVE_ACCEPTANCE=1 --env "BUTLER_DELEGATION_NATIVE_RELEASE=$BUTLER_DELEGATION_NATIVE_RELEASE" \
  --env BUTLER_DELEGATION_NATIVE_CORPUS=/src/scripts/dev/fixtures/delegation-high-conflict-20261012.json \
  --env BUTLER_DELEGATION_NATIVE_IMPORT_FILE=/home/butler/.gemini/antigravity-cli/butler-native-import.json \
  --env BUTLER_DELEGATION_NATIVE_WORKER_BINARY=/binaries/orchestrator.test --env BUTLER_DELEGATION_BROKER_BINARY=/binaries/broker.test \
  --env BUTLER_DELEGATION_NATIVE_REPORT=/private/scenario.json \
  golang:1.27.2-bookworm /binaries/orchestrator.test -test.v -test.run='^TestFullHighDelegationActualAcceptanceIntegration$' -test.timeout=19m >/dev/null
created_runtime=true
docker network connect butler-lab_backend "$name-runtime"
# A single invocation. Nonzero status is retained, never retried.
check_tested_source || exit 2
printf '%s\n' "$proof" > "$window/proof-ref"
status=0
docker start -a "$name-runtime" > "$proof/native-events.txt" 2>&1 || status=$?
docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/private:ro" alpine:3.22 \
  cat /private/scenario.json > "$proof/scenario.json" || status=1
sha256sum "$proof/scenario.json" "$proof/native-events.txt" > "$proof/native-evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"experiment_passed","historical_native_attempts":27,"maximum_total_native_attempts":39,"maximum_new_native_attempts":12,"issue_closure":"pending_audit"}\n'

#!/usr/bin/env bash
# Native spending guard preparation; actual execution remains explicitly gated.
set -euo pipefail
root=${1:?isolated agent root required}
phase=${2:-prepare}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$phase" == prepare || "$phase" == execute ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/app/delegation_native_limits_test.go" ]] || exit 2
if [[ "$phase" == execute ]]; then
  if [[ ${BUTLER_DELEGATION_NATIVE_ACCEPTANCE:-} != 1 ]]; then
    printf '{"status":"skipped","reason":"native_opt_in_unset","native_attempts":0}\n'
    exit 0
  fi
  # Execution wiring is supplied only after the final composed commit review.
  printf '{"status":"pending","reason":"composed_native_fixture_not_ready","native_attempts":0}\n' >&2
  exit 1
fi
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/delegation-native-prepare-$run_id"
name="butler-delegation-native-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$root/.cache/delegation-native-tmp"
created_network=false
created_pg=false
cleanup() {
  $created_pg && docker logs --tail 40 "$name" > "$proof/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export DOCKER_CONFIG="$root/.cache/docker"
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
printf 'Native guard preparation evidence: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/delegation-native-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:delegation-fixture@delegation-postgres:5432/butler_delegation_native_test?sslmode=disable' \
  golang:1.27.2-bookworm go test -json -race -run '^TestNativeDelegation(ObjectiveFixtureRejectsNarrativeAuthority|TwelveAttemptBoundIntegration|PrivateRuntimeFiles|EncryptedFixtureImportWithoutInferenceIntegration)$' \
  -count=1 -timeout=60s ./apps/orchestrator/internal/app > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
printf '{"status":"preparation_passed","native_attempts":0,"model_acceptance":"pending"}\n'

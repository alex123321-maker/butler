#!/usr/bin/env bash
# One promoted task, native High only, baseline41 -> at most45. No retries.
set -euo pipefail
root=${1:?isolated agent root required}
phase=${2:-prepare}
[[ "$root" == /srv/storage/butler/agents/initiative-high-20261012 ]] || exit 2
[[ "$phase" == prepare || "$phase" == execute ]] || exit 2
[[ ${BUTLER_INITIATIVE_HIGH_RELEASE:-} =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
manifest() { (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | sort -z | xargs -0 sha256sum); }
same_source() { manifest | cmp -s "$root/tested-initiative-high-manifest" -; }
window=/srv/storage/butler/native-acceptance-windows/high-initiative-20261012-baseline41
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
if [[ "$phase" == execute ]]; then
  [[ ${BUTLER_INITIATIVE_HIGH_ACCEPTANCE:-} == 1 ]] || exit 2
  [[ ! -e "$window" && ! -L "$window" ]] || { printf '{"status":"denied","reason":"window_already_attempted","new_native_attempts":0}\n'; exit 2; }
  [[ "$(cat "$root/tested-initiative-high-source")" == "$source_dir" ]]
  [[ "$(cat "$root/tested-initiative-high-release")" == "$BUTLER_INITIATIVE_HIGH_RELEASE" ]]
  same_source
  # Global one-use marker seals this invocation before TLS or auth access.
  mkdir -p /srv/storage/butler/native-acceptance-windows
  mkdir "$window"
  printf '%s\n' "$source_dir" > "$window/source-ref"
  printf '%s\n' "$BUTLER_INITIATIVE_HIGH_RELEASE" > "$window/release-ref"
fi
stamp="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/initiative-high-$phase-$stamp"
name="butler-initiative-high-$stamp"
network="$name-net"
mkdir -p "$proof/postgres" "$proof/binaries" "$proof/runtime" "$root/.cache/private-tmp"
chmod 755 "$proof/binaries"
created_pg=false; created_net=false; created_runtime=false
cleanup() {
  $created_runtime && docker rm -f "$name-runtime" >/dev/null 2>&1 || true
  $created_pg && docker logs --tail 30 "$name" > "$proof/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
if [[ "$phase" == execute ]]; then
  sha256sum -c "$root/tested-initiative-high-tls.sha256" >/dev/null
  [[ $(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1) == true ]]
  # Fixed daily endpoint, two authless DNS/CONNECT/TLS lanes, 15s bound.
  # The prerequisite sees no source, profile, token, settings or auth mount.
  tls_status=0
  docker run --rm --network butler-lab_backend --user 10001:10001 --read-only --cap-drop ALL \
    --memory 64m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
    --volume "$root/tested-initiative-high-tls:/prerequisite:ro" \
    golang:1.27.2-bookworm /prerequisite > "$proof/tls-prerequisite.json" || tls_status=$?
  sha256sum "$proof/tls-prerequisite.json" > "$proof/tls-prerequisite.sha256"
  [[ "$tls_status" == 0 ]] || { printf '{"status":"denied","reason":"tls_prerequisite_failed","new_native_attempts":0}\n'; exit 2; }
  import_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
  import_file="$import_parent/butler-native-import.json"
  docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL \
    --memory 32m --pids-limit 16 --cpus 1 --security-opt no-new-privileges:true \
    --volume "$import_parent:/metadata:ro" alpine:3.22 sh -eu -c '
approved=/metadata/butler-native-import.json
test -f "$approved"; test ! -L "$approved"
test "$(stat -c "%u:%a" "$approved")" = 10001:600
size=$(stat -c %s "$approved"); test "$size" -gt 0; test "$size" -le 65536'
fi
docker network create --internal "$network" >/dev/null
created_net=true
docker run -d --name "$name" --runtime runc --network "$network" --network-alias initiative-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=initiative-fixture --env POSTGRES_DB=butler_initiative_high_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$name" pg_isready -U fixture -d butler_initiative_high_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1; sleep 1
done
printf 'Private initiative %s evidence: %s\n' "$phase" "$proof"
manifest > "$proof/source.sha256"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
  --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$proof/binaries:/binaries" --volume "$root/.cache/private-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:initiative-fixture@initiative-postgres:5432/butler_initiative_high_test?sslmode=disable' \
  --env 'BUTLER_TEST_INITIATIVE_POSTGRES_URL=postgres://fixture:initiative-fixture@initiative-postgres:5432/butler_initiative_high_test?sslmode=disable' \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:initiative-fixture@initiative-postgres:5432/butler_initiative_high_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 ./scripts/dev/delegation-final-tls
go build -o /binaries/tls-prerequisite ./scripts/dev/delegation-final-tls
go build -race -p 1 -trimpath -o /binaries/tool-broker ./apps/tool-broker
go test -c -race -p 1 -o /binaries/tool-extension.test ./apps/tool-extension/internal/runtime
go test -c -race -p 1 -o /binaries/orchestrator.test ./apps/orchestrator/internal/app
chmod 555 /binaries/*
export BUTLER_EXTENSION_E2E=1 BUTLER_BROKER_E2E_BINARY=/binaries/tool-broker BUTLER_EXTENSION_E2E_BINARY=/binaries/tool-extension.test
go test -json -race -p 1 -count=1 -timeout=4m -run "^Test(InitiativeProjectFinalProjection|InitiativeHigh(History|LimitIntegration|RevalidationIntegration|PreservesSpentDelegationWindowIntegration|ExpiryAfterRowWaitIntegration|ProviderRejectsUnownedRunIntegration)|NativeDelegationPrivateRuntimeFiles)$" ./apps/orchestrator/internal/app
# The real-process fixture intentionally resolves migrations from repository root.
go tool test2json -t -p github.com/butler/butler/apps/orchestrator/internal/app /binaries/orchestrator.test -test.v -test.timeout=4m -test.run "^TestInitiativeProjectChecksRealBrokerE2E$"
go test -json -race -p 1 -count=1 -timeout=3m ./apps/orchestrator/internal/initiative
go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/initiative ./scripts/dev/delegation-final-tls
' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl" || exit 1
docker run --rm --network none --user 0:0 --volume "$proof/runtime:/private" alpine:3.22 \
  sh -eu -c 'chown 10001:10001 /private; chmod 700 /private'
docker run --rm --network none --user 10001:10001 --read-only --cap-drop ALL \
  --memory 512m --pids-limit 64 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" --env TMPDIR=/private \
  golang:1.27.2-bookworm /binaries/orchestrator.test -test.v -test.run='^TestNativeDelegationPrivateRuntimeFiles$' > "$proof/private-files.txt"
if [[ "$phase" == prepare ]]; then
  manifest | cmp -s "$proof/source.sha256" -
  cp "$proof/binaries/tls-prerequisite" "$root/tested-initiative-high-tls"
  chmod 555 "$root/tested-initiative-high-tls"
  sha256sum "$root/tested-initiative-high-tls" > "$root/tested-initiative-high-tls.sha256"
  cp "$proof/source.sha256" "$root/tested-initiative-high-manifest"
  printf '%s\n' "$source_dir" > "$root/tested-initiative-high-source"
  printf '%s\n' "$BUTLER_INITIATIVE_HIGH_RELEASE" > "$root/tested-initiative-high-release"
  printf '{"status":"preparation_passed","native_calls":0,"baseline":41,"maximum_new_attempts":4}\n'
  exit 0
fi
docker create --name "$name-runtime" --runtime runc --user 10001:10001 --network "$network" \
  --memory 1536m --pids-limit 160 --cpus 2 --read-only --cap-drop ALL --security-opt no-new-privileges:true --workdir /src \
  --volume "$source_dir:/src:ro" --volume "$proof/binaries:/binaries:ro" --volume "$proof/runtime:/private" \
  --volume "$import_file:/home/butler/.gemini/antigravity-cli/butler-native-import.json:ro" \
  --env TMPDIR=/private --env HOME=/home/butler \
  --env HTTPS_PROXY=http://antigravity-proxy:18080 --env HTTP_PROXY=http://antigravity-proxy:18080 --env NO_PROXY=localhost,127.0.0.1,initiative-postgres \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:initiative-fixture@initiative-postgres:5432/butler_initiative_high_test?sslmode=disable' \
  --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:initiative-fixture@initiative-postgres:5432/butler_initiative_high_test?sslmode=disable' \
  --env BUTLER_EXTENSION_E2E=1 --env BUTLER_BROKER_E2E_BINARY=/binaries/tool-broker --env BUTLER_EXTENSION_E2E_BINARY=/binaries/tool-extension.test \
  --env BUTLER_INITIATIVE_HIGH_ACCEPTANCE=1 --env "BUTLER_INITIATIVE_HIGH_RELEASE=$BUTLER_INITIATIVE_HIGH_RELEASE" \
  --env BUTLER_INITIATIVE_HIGH_IMPORT_FILE=/home/butler/.gemini/antigravity-cli/butler-native-import.json \
  --env BUTLER_INITIATIVE_HIGH_REPORT=/private/scenario.json \
  golang:1.27.2-bookworm /binaries/orchestrator.test -test.v -test.run='^TestInitiativeHighActualAcceptanceE2E$' -test.timeout=12m >/dev/null
created_runtime=true
docker network connect butler-lab_backend "$name-runtime"
same_source
printf '%s\n' "$proof" > "$window/proof-ref"
status=0
docker start -a "$name-runtime" > "$proof/native-events.txt" 2>&1 || status=$?
docker run --rm --network none --user 10001:10001 --volume "$proof/runtime:/private:ro" alpine:3.22 \
  cat /private/scenario.json > "$proof/scenario.json" || status=1
sha256sum "$proof/scenario.json" "$proof/native-events.txt" > "$proof/native-evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
printf '{"status":"experiment_passed","maximum_total_attempts":45,"maximum_new_attempts":4,"issue_closure":"pending_audit"}\n'

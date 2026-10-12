#!/usr/bin/env bash
# Prospective separate High scope. Preparation uses only private PG/synthetic
# providers; unknown V2 review bytes deny execution before auth/network/markers.
set -euo pipefail
root=${1:?isolated root required}; phase=${2:-prepare}; release=${3:?exact clean release required}
[[ "$root" == /srv/storage/butler/agents/scenario-release-chain-high-20261012 && "$phase" =~ ^(prepare|execute)$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
proof="$root/reports/chain-high-$phase-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof/bin" /srv/storage/butler/.cache/go-tmp
printf '%s\n' "$release" > "$proof/source-commit.txt"
manifest() { (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | sort -z | xargs -0 sha256sum); }
manifest > "$proof/source.sha256"
printf 'Private prospective High-chain %s proof: %s\n' "$phase" "$proof"
if [[ "$phase" == execute ]]; then
  [[ -f "$root/prepared-release" && "$(cat "$root/prepared-release")" == "$release" && "$(cat "$root/prepared-source")" == "$source_dir" ]]
  manifest | cmp -s "$root/prepared-source.sha256" -
  sha256sum -c "$root/prepared-runtime.sha256" >/dev/null
  prepared=$(cat "$root/prepared-proof")
  [[ "$prepared" == "$root/reports/chain-high-prepare-"* ]]
  # Deliberately first: no provider import, TLS, backend network, persistent
  # native scope or old-window read/write can precede this frozen source gate.
  docker run --rm --runtime runc --pull never --network none --read-only --user "$(id -u):$(id -g)" --cap-drop ALL --security-opt no-new-privileges:true \
    --memory 512m --pids-limit 64 --volume "$source_dir:/src:ro" --volume "$prepared/bin:/bin-proof:ro" --workdir /src \
    --env BUTLER_SCENARIO_CHAIN_HIGH_GATE=1 golang:1.27.2-bookworm \
    /bin-proof/app.test -test.v -test.timeout=30s -test.run='^TestReleaseChainHighOperationalGate$' > "$proof/prerequisite-gate.txt" 2>&1
  window=/srv/storage/butler/native-acceptance-windows/issue33-high-release-chain-20261012-baseline480
  [[ ! -e "$window" && ! -L "$window" ]] || { printf 'Denied: sealed separate native window.\n' >&2; exit 2; }
  [[ "$(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1)" == true ]]
  docker run --rm --runtime runc --pull never --network butler-lab_backend --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --memory 64m --pids-limit 16 \
    --volume "$prepared/bin/tls-prerequisite:/tls:ro" golang:1.27.2-bookworm /tls > "$proof/tls.json"
  import_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
  docker run --rm --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --volume "$import_parent:/metadata:ro" alpine:3.21 sh -eu -c 'p=/metadata/butler-native-import.json; test -f "$p"; test ! -L "$p"; test "$(stat -c "%u:%a" "$p")" = 10001:600; size=$(stat -c %s "$p"); test "$size" -gt 0; test "$size" -le 65536'
  mkdir -p /srv/storage/butler/native-acceptance-windows
  mkdir "$window"
  printf '%s\n' "$release" > "$window/release-ref"
  printf '%s\n' "$source_dir" > "$window/source-ref"
  printf '%s\n' "$proof" > "$window/preflight-proof-ref"
  export BUTLER_SCENARIO_CHAIN_HIGH_SEALED_ENTRY=1
  exec bash "$source_dir/scripts/dev/scenario-release-chain-process-lab.sh" "$root" high "$release"
fi
name="butler-chain-high-prepare-$(date -u +%Y%m%dT%H%M%SZ)-$$"; network="$name-net"; pg="$name-pg"
created_network=false; created_pg=false
cleanup() { $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true; $created_network && docker network rm "$network" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
build=(docker run --rm --runtime runc --pull never --network none --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true
  --volume "$source_dir:/src:ro" --volume "$proof/bin:/bin-proof" --volume /srv/storage/butler/.cache:/cache --workdir /src
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp)
"${build[@]}" golang:1.27.2-bookworm bash -eu -c '
go test -c -race -o /bin-proof/app.test ./apps/orchestrator/internal/app
go build -o /bin-proof/test2json cmd/test2json
CGO_ENABLED=0 go build -trimpath -o /bin-proof/tool-broker ./apps/tool-broker
CGO_ENABLED=0 go build -trimpath -o /bin-proof/tool-converter ./apps/tool-converter
go test -c -o /bin-proof/extension.test ./apps/tool-extension/internal/runtime
go build -o /bin-proof/tls-prerequisite ./scripts/dev/delegation-final-tls
go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator
chmod 555 /bin-proof/*
' > "$proof/build-vet.log" 2>&1
docker network create --internal "$network" >/dev/null; created_network=true
docker run -d --runtime runc --name "$pg" --network "$network" --network-alias chain-prepare-postgres --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=chain-prepare-fixture --env POSTGRES_DB=butler_chain_prepare_test \
  pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB >/dev/null; created_pg=true
for attempt in $(seq 1 60); do docker exec "$pg" pg_isready -U fixture -d butler_chain_prepare_test >/dev/null 2>&1 && break; [[ "$attempt" != 60 ]] || exit 1; sleep 1; done
docker run --rm --runtime runc --pull never --network "$network" --read-only --user "$(id -u):$(id -g)" --cap-drop ALL --security-opt no-new-privileges:true --memory 1536m --pids-limit 160 --cpus 2 \
  --tmpfs /tmp:rw,nosuid,nodev,size=64m --volume "$source_dir:/src:ro" --volume "$proof/bin:/bin-proof:ro" --workdir /src \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:chain-prepare-fixture@chain-prepare-postgres:5432/butler_chain_prepare_test?sslmode=disable' \
  --entrypoint /bin-proof/test2json golang:1.27.2-bookworm -t -p github.com/butler/butler/apps/orchestrator/internal/app \
  /bin-proof/app.test -test.v=test2json -test.timeout=3m '-test.run=^TestReleaseChainHigh(Window|AllFailedEntries|TerminalMetadata|OperationalPrerequisite)' > "$proof/tests.jsonl"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
required=(TestReleaseChainHighOperationalPrerequisiteIsNotApproved TestReleaseChainHighWindowSharedCeilingRestartAndExpiryIntegration TestReleaseChainHighWindowChecksDeadlineAfterCounterWaitIntegration TestReleaseChainHighWindowSharesCounterAcrossPrivateTaskSchemasIntegration TestReleaseChainHighAllFailedEntriesSpentButSourceDenialZeroIntegration TestReleaseChainHighTerminalMetadataRetainedAndReleaseGatedIntegration)
for case_name in failed_terminal_known untrusted_diagnostic_type conflicting_complete_usage true_zero_triple missing_total_legacy_pair missing_metadata missing_version incomplete_final_stream completed_final; do required+=("TestReleaseChainHighTerminalMetadataRetainedAndReleaseGatedIntegration/$case_name"); done
for test_name in "${required[@]}"; do
  [[ "$(awk -v needle="\"Test\":\"$test_name\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$proof/tests.jsonl")" == 1 ]]
done
manifest | cmp -s "$proof/source.sha256" -
sha256sum "$proof/bin/"* > "$root/prepared-runtime.sha256"
cp "$proof/source.sha256" "$root/prepared-source.sha256"
printf '%s\n' "$source_dir" > "$root/prepared-source"
printf '%s\n' "$proof" > "$root/prepared-proof"
printf '%s\n' "$release" > "$root/prepared-release"
sha256sum "$proof/tests.jsonl" "$proof/source.sha256" > "$proof/evidence.sha256"
printf '{"status":"offline_preparation_passed","native_calls":0,"baseline480":"prospective_pending_V2","maximum_new_attempts_design":40,"execution_enabled":false}\n' > "$proof/summary.json"
printf 'PASS: offline prospective cap/current proof tests; native execution remains denied.\n'

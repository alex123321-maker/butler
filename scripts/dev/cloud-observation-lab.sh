#!/usr/bin/env bash
# Independent cloud scorer tests only. No auth, import, provider or network.
set -euo pipefail
root=${1:?isolated agent root required}
phase=${2:-narrow}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$phase" == narrow || "$phase" == broad || "$phase" == v2 ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/internal/acceptance/cloud_observation.go" ]] || exit 2
umask 077
proof="$root/reports/cloud-scorer-$phase-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof" "$root/.cache/go-race" "$root/.cache/go-mod" "$root/.cache/go-tmp" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
printf 'Private zero-inference cloud scorer proof: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume "$root/.cache:/cache" \
  --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env CLOUD_PHASE="$phase" golang:1.27.2-bookworm bash -eu -c '
if [[ "$CLOUD_PHASE" == v2 ]]; then
  go test -json -race -p 1 -count=1 -run "^TestCloudV2" ./internal/acceptance ./cmd/cloud-observe
elif [[ "$CLOUD_PHASE" == narrow ]]; then
  go test -json -race -p 1 -count=1 -run "^TestCloud" ./internal/acceptance
else
  go test -json -race -p 1 -count=1 ./internal/acceptance ./cmd/command-observe ./cmd/cloud-observe
fi
go vet ./internal/acceptance ./cmd/acceptance ./cmd/command-observe ./cmd/cloud-observe
go run ./cmd/cloud-observe -mode policy -root /src > /cache/frozen-cloud-policy.json
go run ./cmd/cloud-observe -mode inputs -root /src > /cache/cloud-inputs.json
' > "$proof/tests.jsonl" 2> "$proof/diagnostics.log"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
printf '{"status":"cloud_scorer_fixture_passed","model_calls":0,"native_quality_claim":false}\n'

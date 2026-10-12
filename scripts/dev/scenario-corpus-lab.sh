#!/usr/bin/env bash
# Offline command corpus only: no provider, credential, model or shared service.
set -euo pipefail
root=${1:?isolated agent root required}
phase=${2:-narrow}
commit=${3:?exact candidate commit required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$phase" == narrow || "$phase" == broad ]] || exit 2
[[ "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/testdata/acceptance/ru-command-proposals-v1.json" ]] || exit 2
umask 077
proof="$root/reports/corpus-$phase-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof" "$root/.cache/go-race" "$root/.cache/go-mod" "$root/.cache/go-tmp" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
printf 'Private command corpus proof: %s\n' "$proof"
(cd "$source_dir"; find . -type f -print0 | sort -z | xargs -0 sha256sum) > "$proof/source-files.sha256"
sha256sum "$proof/source-files.sha256" > "$proof/source-manifest.sha256"
jq -n --arg code_commit "$commit" --arg source_snapshot "$source_dir" \
  --arg kernel "$(uname -r)" --arg image_id "$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')" \
  --argjson cpu_count "$(getconf _NPROCESSORS_ONLN)" --argjson memory_total_kib "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" \
  '{code_commit:$code_commit,source_snapshot:$source_snapshot,kernel:$kernel,image_id:$image_id,cpu_count:$cpu_count,memory_total_kib:$memory_total_kib,limits:{cpus:2,memory_bytes:1610612736,pids:128,network:"none",source:"read_only"},evidence_kind:"fixture_validation",model_calls:0}' > "$proof/hardware.json"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume "$root/.cache:/cache" --volume "$proof:/proof" \
  --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env CORPUS_PHASE="$phase" --env CORPUS_CODE_COMMIT="$commit" --env CORPUS_PROOF_DIR=/proof golang:1.27.2-bookworm bash -eu -c '
if [[ "$CORPUS_PHASE" == narrow ]]; then
  go test -json -race -p 1 -count=1 -run "^Test(SupplementaryCommand|CommandWitness|CommandObserver)" ./internal/acceptance
else
  go test -json -race -p 1 -count=1 ./internal/acceptance ./cmd/command-observe
fi
go vet ./internal/acceptance ./cmd/acceptance ./cmd/command-observe
go run ./cmd/command-observe -mode inputs -root /src -split holdout > /cache/command-inputs.json
! grep -q SYNTHETIC_SECRET /cache/command-inputs.json
cp /cache/command-inputs.json /proof/command-inputs.json
' > "$proof/tests.jsonl" 2> "$proof/diagnostics.log"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
printf '{"status":"offline_corpus_harness_passed","model_calls":0,"golden_review":"candidate_requires_independent_review","model_quality_evidence":false}\n'

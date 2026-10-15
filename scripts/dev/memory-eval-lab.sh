#!/usr/bin/env bash
# Offline #78 harness validation in a fresh agent snapshot; zero model calls.
set -euo pipefail
root=${1:?isolated agent root required}
commit=${2:?exact candidate commit required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/testdata/acceptance/ru-memory-v1.json" ]] || exit 2
umask 077
proof="$root/reports/memory-eval-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof" "$root/.cache/go-race" "$root/.cache/go-tmp" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
image_id=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
jq -n --arg code_commit "$commit" --arg kernel "$(uname -r)" --arg image_id "$image_id" \
  --argjson cpu_count "$(getconf _NPROCESSORS_ONLN)" \
  --argjson memory_total_kib "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" \
  '{code_commit:$code_commit,kernel:$kernel,image_id:$image_id,cpu_count:$cpu_count,memory_total_kib:$memory_total_kib,limits:{cpus:2,memory_bytes:1610612736,pids:128,network:"none",source:"read_only"},evidence_kind:"fixture_validation",model_calls:0}' > "$proof/hardware.json"
printf 'Private memory-eval proof: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" \
  --volume "$root/.cache:/cache" --volume /srv/storage/butler/.cache/go-mod:/gomod:ro --volume "$proof:/proof" \
  --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/gomod --env GOTMPDIR=/cache/go-tmp \
  --env MEMORY_CODE_COMMIT="$commit" golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 -run "^TestMemory" ./internal/acceptance ./cmd/memory-eval > /proof/narrow-tests.jsonl
go vet ./internal/acceptance ./cmd/memory-eval
go test -json -race -p 1 -count=1 -timeout 20m ./internal/acceptance ./cmd/memory-eval > /proof/regression-tests.jsonl
go run ./cmd/memory-eval -mode fixture -root /src -hardware /proof/hardware.json \
  -code-commit "$MEMORY_CODE_COMMIT" -window-id "fixture-$MEMORY_CODE_COMMIT" -output-dir /proof/fixture
' > "$proof/runner.jsonl" 2> "$proof/diagnostics.log"
jq '{status,arms,promotion_allowed,model_quality_evidence,execution_authorized,reasons}' "$proof/fixture/report.json" > "$proof/summary.json"
(cd "$proof"; find . -type f ! -name artifacts.sha256 -print0 | sort -z | xargs -0 sha256sum) > "$proof/artifacts.sha256"
printf 'Memory-eval fixture complete: %s\n' "$proof"

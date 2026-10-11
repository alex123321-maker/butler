#!/usr/bin/env bash
# Public corpus/harness proof only; no model, credentials or shared deployment.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/testdata/acceptance/ru-scenarios-v3.json" ]] || exit 2
umask 077
proof="$root/reports/corpus-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof" /srv/storage/butler/.cache/go-tmp
export DOCKER_CONFIG="$root/.cache/docker"
printf 'Private corpus proof: %s\n' "$proof"
docker run --rm --user "$(id -u):$(id -g)" --network none --memory 1536m --pids-limit 128 --cpus 2 \
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --workdir /src --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 ./internal/acceptance ./cmd/acceptance
go vet ./internal/acceptance ./cmd/acceptance
go run ./cmd/acceptance -mode corpus -root /src' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
printf '{"status":"corpus_harness_passed","model_calls":0,"independent_review":"pending"}\n'

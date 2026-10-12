#!/usr/bin/env bash
# Exact synthetic High wire and immutable failed-spend reader; no auth/network.
set -euo pipefail
root=${1:?isolated root required}
[[ "$root" == /srv/storage/butler/agents/scenario-high-generation ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
proof="$root/reports/generation-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof" /srv/storage/butler/.cache/go-tmp
(cd "$source_dir"; find . -type f -print0 | sort -z | xargs -0 sha256sum) > "$proof/source.sha256"
docker run --rm --runtime runc --pull never --user "$(id -u):$(id -g)" --network none \
 --read-only --cap-drop ALL --security-opt no-new-privileges:true --memory 1536m --cpus 2 --pids-limit 160 \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/cache/go-tmp \
 golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 ./internal/transport ./internal/transport/antigravity
go test -json -race -p 1 -count=1 -run "^TestScenarioShadowHistory" ./apps/orchestrator/internal/app
go vet ./internal/transport ./internal/transport/antigravity ./apps/orchestrator/internal/app
' > "$proof/tests.jsonl" 2> "$proof/diagnostics.log"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" "$proof/source.sha256"
printf 'Generation/history proof: %s\n' "$proof"

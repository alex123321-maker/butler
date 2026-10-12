#!/usr/bin/env bash
# Private fixed-stage diagnostics only; synthetic providers and no credentials.
set -euo pipefail
root=${1:?isolated root required}; mode=${2:-narrow}
[[ "$root" == /srv/storage/butler/agents/scenario-high-private-stages && "$mode" =~ ^(narrow|all)$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
if [[ "$mode" == all ]]; then exec bash "$source_dir/scripts/dev/tool-turn-usage-lab.sh" "$root" all; fi
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
proof="$root/reports/provider-stages-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof" /srv/storage/butler/.cache/go-tmp
(cd "$source_dir"; find . -type f -print0 | sort -z | xargs -0 sha256sum) > "$proof/source.sha256"
docker run --rm --runtime runc --pull never --network none --user "$(id -u):$(id -g)" --read-only --cap-drop ALL --security-opt no-new-privileges:true --memory 1536m --cpus 2 --pids-limit 160 \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/cache/go-tmp \
 golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 -run "^TestProvider(StageObserver|DiagnosticStage)" ./internal/transport ./internal/transport/antigravity
go vet ./internal/transport ./internal/transport/antigravity
' > "$proof/tests.jsonl" 2> "$proof/diagnostics.log"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
for test in TestProviderStageObserverIsRequestLocalAndContentFree TestProviderDiagnosticStageSeparatesAuthHTTPAndStreamFailure; do
  [[ $(grep -c '"Action":"pass".*"Test":"'"$test"'"' "$proof/tests.jsonl") == 1 ]]
done
sha256sum "$proof/tests.jsonl" "$proof/source.sha256"
printf 'Private phase proof: %s\n' "$proof"

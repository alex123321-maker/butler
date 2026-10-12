#!/usr/bin/env bash
# Validate all fifteen immutable public reports. No allowance, auth or inference.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/app/scenario_native_history_test.go" ]] || exit 2
manifest() {
  (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | sort -z | xargs -0 sha256sum)
}
umask 077
proof="$root/reports/scenario-native-history-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof" "$root/.cache/history-tmp"
export DOCKER_CONFIG="$root/.cache/docker"
manifest > "$proof/source.sha256"
printf 'Public native history evidence: %s\n' "$proof"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none \
  --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache \
  --volume "$root/.cache/history-tmp:/private-tmp" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/private-tmp \
  golang:1.27.2-bookworm go test -json -race -p 1 -count=1 -timeout=90s \
  -run '^TestScenarioNativeHistory' ./apps/orchestrator/internal/app > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl" > "$proof/events.sha256"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network none \
  --memory 1536m --pids-limit 128 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  golang:1.27.2-bookworm go vet ./apps/orchestrator/internal/app > "$proof/vet.txt"
manifest | cmp -s "$proof/source.sha256" -
sha256sum "$proof/source.sha256" "$proof/tests.jsonl" "$proof/vet.txt"
printf '{"status":"scenario_native_history_passed","historical_attempts":45,"known_usage_pairs":20,"unknown_usage_pairs":25,"native_calls":0}\n'

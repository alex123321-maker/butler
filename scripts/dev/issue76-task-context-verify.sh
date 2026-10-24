#!/usr/bin/env bash
# Offline task-goal wiring verification; no native auth or inference.
set -euo pipefail
source_root=${1:?isolated source root}
root=${2:?fresh verification root}
subnet=${3:?unused private CIDR}
cache=${4:-$root/cache}
[[ "$source_root" =~ ^/srv/storage/butler/agents/issue76-source-[a-zA-Z0-9_-]+$ && "$root" =~ ^/srv/storage/butler/agents/issue76-verify-[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$subnet" =~ ^10\.254\.76\.[0-9]+/28$ && ! -e "$root" ]] || exit 2
[[ "$cache" =~ ^/srv/storage/butler/agents/issue76-verify-[a-zA-Z0-9_-]+/cache$ ]] || exit 2
src=$(readlink -f "$source_root/current")
[[ "$src" == "$source_root/snapshots/"* ]] || exit 2
umask 077
mkdir -p "$root/build" "$cache/go-build" "$cache/go-tmp"
image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
printf '%s\n' "$src" > "$root/source-path"
printf '%s\n' "$image" > "$root/image"
docker run --rm --network none --cpus 2 --memory 1536m --pids-limit 128 \
 -v "$src:/src:ro" -v "$cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$root/build:/build" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod "$image" \
 go test -c -race -p 1 -o /build/orchestrator.test ./apps/orchestrator/internal/orchestrator > "$root/build.log" 2>&1
sha256sum "$root/build/orchestrator.test" > "$root/binary.sha256"
tag=$(basename "$root")
docker network create --internal --subnet "$subnet" "$tag" > "$root/network-id"
docker run -d --name "$tag-pg" --network "$tag" --network-alias verification-postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$root/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=issue76_wiring_test pgvector/pgvector:pg17 > "$root/postgres-id"
trap 'docker stop "$tag-pg" >/dev/null' EXIT
ready=false
for _ in $(seq 1 30); do
 if docker exec "$tag-pg" pg_isready -U butler -d issue76_wiring_test >/dev/null; then ready=true; break; fi
 sleep 1
done
[[ "$ready" == true ]] || exit 2
chmod 755 "$root" "$root/build" "$root/build/orchestrator.test"
docker run --rm --user 10001:10001 --network "$tag" --read-only --cap-drop ALL --security-opt no-new-privileges:true --cpus 2 --memory 1536m --pids-limit 128 \
 --tmpfs /tmp:rw,noexec,nosuid,size=134217728 -v "$src:/src:ro" -v "$root/build:/runtime:ro" -w /src/apps/orchestrator/internal/orchestrator \
 -e BUTLER_TEST_POSTGRES_URL='postgres://butler@verification-postgres:5432/issue76_wiring_test?sslmode=disable' \
 "$image" /runtime/orchestrator.test -test.run '^Test(DurableTaskExecutionIntegration|ProfileSelectionUsesOnlyCurrentValidatedTaskGoal|AssignedLocalOnlyTaskRetainsLocalQueryAndRechecksAuthority|OptionalContextTrimsOneItemAndPreservesDocument)$' -test.v -test.timeout 5m > "$root/result.log" 2>&1
! grep -q -- '--- SKIP:' "$root/result.log"
cat "$root/result.log"

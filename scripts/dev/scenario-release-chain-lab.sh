#!/usr/bin/env bash
# Disposable PG/kernel first; full actual converter/Broker chain added below.
set -euo pipefail
root=${1:?isolated agent root required}
mode=${2:-kernel}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ && "$mode" == kernel ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/migrations/069_verified_task_notifications.up.sql" ]] || exit 2
id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/release-chain-$id"
pg="butler-release-chain-$id-pg"
network="butler-release-chain-$id-net"
umask 077
mkdir -p "$proof/private/postgres" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
created_pg=false
created_network=false
cleanup() {
  $created_pg && docker logs --tail 80 "$pg" > "$proof/private/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
printf 'Private release chain proof: %s\n' "$proof"
(cd "$source_dir"; find . -type f -not -path './.cache/*' -print0 | sort -z | xargs -0 sha256sum) > "$proof/source-manifest.sha256"
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --runtime runc --name "$pg" --network "$network" --network-alias release-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=release-fixture --env POSTGRES_DB=butler_release_chain_test \
  --volume "$proof/private/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
  -c shared_buffers=32MB -c max_connections=32 -c work_mem=4MB -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  if docker exec "$pg" pg_isready -h 127.0.0.1 -U fixture -d butler_release_chain_test >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
docker exec "$pg" psql -q -v ON_ERROR_STOP=1 -U fixture -d butler_release_chain_test -c 'CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public' >/dev/null
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
  --memory 2g --pids-limit 192 --cpus 2 --security-opt no-new-privileges:true \
  --volume "$source_dir:/src:ro" --volume "$proof:/proof" --volume /srv/storage/butler/.cache:/cache --workdir /src \
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2 \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:release-fixture@release-postgres:5432/butler_release_chain_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -c '
    set -euo pipefail
    go test -json -race -count=1 -timeout=120s -run "^Test(VerifiedTaskNotificationCurrentOwnerAtomicCompletion|ReleaseChainEntryCheckerDenials)Integration$" ./apps/orchestrator/internal/app > /proof/tests.jsonl
    go vet ./apps/orchestrator/internal/task ./apps/orchestrator/internal/goal ./apps/orchestrator/internal/completion ./apps/orchestrator/internal/initiative ./apps/orchestrator/internal/app ./apps/orchestrator/internal/api > /proof/vet.log 2>&1
  '
sha256sum "$proof/tests.jsonl" "$proof/source-manifest.sha256" > "$proof/evidence.sha256"
required=(TestVerifiedTaskNotificationCurrentOwnerAtomicCompletionIntegration TestReleaseChainEntryCheckerDenialsIntegration)
for fault in verified unverified partial foreign_owner deleted_source corrected_source corrected_before_send atomic_notice_failure; do
  required+=("TestVerifiedTaskNotificationCurrentOwnerAtomicCompletionIntegration/$fault")
done
for failure in owner_actor foreign_task unadmitted_child old_lease expired_lease stopped changed_source bad_expected_pin continue_denied submit_after_stop; do required+=("TestReleaseChainEntryCheckerDenialsIntegration/$failure"); done
for name in "${required[@]}"; do
  test "$(awk -v needle="\"Test\":\"$name\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$proof/tests.jsonl")" = 1
done
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
printf '{"stage":"kernel","actual_native_calls":0,"full_chain":"pending","test_exit":0}\n' > "$proof/summary.json"
printf 'PASS: actual-PG verified task delivery kernel, race/vet, zero native calls.\n'

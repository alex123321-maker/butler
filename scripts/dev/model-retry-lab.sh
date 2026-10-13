#!/usr/bin/env bash
# Production worker/Core retry regression; synthetic providers, zero inference.
set -euo pipefail
root=${1:?isolated agent root required}
release=${2:?exact source commit required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/apps/orchestrator/internal/input/model_retry.go" ]] || exit 2
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
proof="$root/reports/model-retry-$(date -u +%Y%m%dT%H%M%SZ)-$$"
name="butler-model-retry-$(date -u +%Y%m%dT%H%M%SZ)-$$"
network="$name-net"; pg="$name-pg"
go_image=golang@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
pg_image=pgvector/pgvector@sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d
mkdir -p "$proof/postgres" /srv/storage/butler/.cache/go-tmp
manifest() { (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | sort -z | xargs -0 sha256sum); }
manifest > "$proof/source.sha256"
printf '%s\n' "$release" > "$proof/source-commit.txt"
created_network=false; created_pg=false
cleanup() { $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true; $created_network && docker network rm "$network" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
docker network create --internal "$network" >/dev/null; created_network=true
docker run -d --runtime runc --pull never --name "$pg" --network "$network" --network-alias retry-postgres \
  --memory 384m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=retry-fixture --env POSTGRES_DB=butler_model_retry_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" "$pg_image" \
  -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB >/dev/null; created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$pg" pg_isready -U fixture -d butler_model_retry_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
docker run --rm --runtime runc --pull never --network "$network" --read-only --user "$(id -u):$(id -g)" \
  --cap-drop ALL --security-opt no-new-privileges:true --memory 1536m --pids-limit 160 --cpus 2 \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --workdir /src \
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=1 \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env TMPDIR=/cache/go-tmp \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:retry-fixture@retry-postgres:5432/butler_model_retry_test?sslmode=disable' \
  "$go_image" bash -eu -c '
go test -race -json -count=1 -timeout=5m -run "^Test(ModelRetry|ModelAdmissionDeadlineOrigin)" ./internal/budget ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/input
go vet ./internal/budget ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/input
' > "$proof/tests.jsonl" 2> "$proof/diagnostics.log"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
mapfile -t required < "$source_dir/scripts/dev/model-retry-required-tests.txt"
[[ "${#required[@]}" == 81 ]]
for test_name in "${required[@]}"; do
  [[ "$test_name" =~ ^Test[A-Za-z0-9_/-]+$ ]]
  [[ "$(awk -v needle="\"Test\":\"$test_name\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$proof/tests.jsonl")" == 1 ]]
done
manifest | cmp -s "$proof/source.sha256" -
passes=$(awk 'index($0,"\"Action\":\"pass\"") && index($0,"\"Test\":") { n++ } END { print n+0 }' "$proof/tests.jsonl")
[[ "$passes" == "${#required[@]}" ]]
sha256sum "$proof/tests.jsonl" "$proof/diagnostics.log" "$proof/source.sha256" > "$proof/evidence.sha256"
printf '{"status":"model_retry_regression_passed","source_commit":"%s","named_passes":%s,"required_top_level_tests":15,"native_calls":0,"historical_backfills":0}\n' "$release" "$passes" > "$proof/summary.json"
printf 'PASS: durable model retry, private proof %s\n' "$proof"

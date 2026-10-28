#!/usr/bin/env bash
set -euo pipefail
root=${1:?isolated agent root}
[[ "$root" =~ ^(/srv/storage/butler/agents/browser-continuation-|/tmp/butler-browser-continuation-)[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
mkdir -p "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/.cache/docker" "$root/reports"
export DOCKER_CONFIG="$root/.cache/docker"
exec 9>"$root/.cache/validation.lock"
flock -n 9
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
pg="butler-stopped-review-pg-$run"
cleanup() {
 docker logs "$pg" >"$root/reports/postgres-$run.log" 2>&1 || true
 docker inspect --format 'test_postgres_oom={{.State.OOMKilled}}' "$pg" || true
 docker rm -f "$pg" >/dev/null 2>&1 || true
}
trap cleanup EXIT
docker run -d --name "$pg" --network none --memory 2g --cpus 1 --pids-limit 128 \
 --tmpfs /var/lib/postgresql/data:rw,size=1536m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=stopped_test "$pg_image" \
 -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB -c checkpoint_timeout=30s >/dev/null
for i in $(seq 1 30); do docker exec "$pg" pg_isready -U postgres -d stopped_test >/dev/null 2>&1 && break; sleep 1; done
docker exec "$pg" pg_isready -U postgres -d stopped_test
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_COMMUNICATION_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/stopped_test?sslmode=disable'
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/stopped_test?sslmode=disable'
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/stopped_test?sslmode=disable'
 -e BUTLER_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/stopped_test?sslmode=disable')
report="$root/reports/stopped-$run.jsonl"
printf 'source=%s go=%s postgres=%s\n' "$src" "$go_image" "$pg_image"
if ! docker run "${args[@]}" "$go_image" go test -race -p 1 -count=1 -json -timeout 8m \
 ./internal/actionauth ./internal/budget ./apps/orchestrator/internal/input ./apps/orchestrator/internal/communication \
 -run '^Test(Stopped|CommunicationCloseFailed)' >"$report"; then
 grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$report" || tail -30 "$report"; exit 1
fi
if grep -q '"Action":"skip"' "$report"; then grep '"Action":"skip"' "$report"; exit 1; fi
grep -q '"Test":"TestStoppedLocalKernelReviewIntegration".*"Action":"pass"\|"Action":"pass".*"Test":"TestStoppedLocalKernelReviewIntegration"' "$report"
docker run "${args[@]}" "$go_image" go vet ./internal/actionauth ./internal/budget ./apps/orchestrator/internal/input ./apps/orchestrator/internal/communication ./apps/orchestrator/cmd/stopped-review
docker run "${args[@]}" -e CGO_ENABLED=0 "$go_image" go build -o /cache/stopped-review ./apps/orchestrator/cmd/stopped-review
sha256sum "$root/.cache/stopped-review"
printf 'Mandatory stopped-review race/integration tests passed without skips; vet/build passed. report=%s\n' "$report"

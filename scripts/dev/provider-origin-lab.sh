#!/usr/bin/env bash
# Private operator audit validation, disposable encrypted PG; zero inference.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
mode=${2:-full}
[[ "$mode" == narrow || "$mode" == full ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/provider-origin-$run_id"
name="butler-provider-origin-$run_id"
network="$name-net"
umask 077
mkdir -p "$proof/postgres" "$root/runtime" /srv/storage/butler/.cache/go-tmp
created_network=false
created_pg=false
cleanup() {
  $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export DOCKER_CONFIG=/srv/storage/butler/.cache/docker
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$name" --runtime runc --network "$network" --network-alias auth-postgres \
  --memory 256m --pids-limit 64 --cpus 1 --security-opt no-new-privileges \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=auth-fixture --env POSTGRES_DB=provider_auth_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" postgres:17 \
  -c shared_buffers=16MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 60); do
  docker exec "$name" pg_isready -U fixture -d provider_auth_test >/dev/null 2>&1 && break
  [[ "$attempt" != 60 ]] || exit 1
  sleep 1
done
printf 'Private origin audit proof: %s\n' "$proof"
docker run --rm --runtime runc --user "$(id -u):$(id -g)" --network "$network" \
  --memory 768m --pids-limit 128 --cpus 2 --security-opt no-new-privileges \
  --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$root/runtime:/out" --workdir /src \
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  --env "BUTLER_ORIGIN_CHECK_MODE=$mode" \
  --env 'BUTLER_TEST_PROVIDER_AUTH_POSTGRES_URL=postgres://fixture:auth-fixture@auth-postgres:5432/provider_auth_test?sslmode=disable' \
  golang:1.27.2-bookworm bash -eu -c '
if [[ "$BUTLER_ORIGIN_CHECK_MODE" == narrow ]]; then
go test -json -race -p 1 -count=1 -run "^TestAntigravity(OriginAudit|ImportEncryptedPostgres)" ./internal/providerauth
else
go test -json -race -p 1 -count=1 ./internal/providerauth ./cmd/provider-auth ./internal/transport/antigravity
go vet ./internal/providerauth ./cmd/provider-auth ./internal/transport/antigravity
CGO_ENABLED=0 GOCACHE=/cache/go-build go build -o /out/provider-auth-origin ./cmd/provider-auth
fi
' > "$proof/tests.jsonl"
sha256sum "$proof/tests.jsonl"
printf '{"status":"provider_origin_validation_passed","model_calls":0}\n'

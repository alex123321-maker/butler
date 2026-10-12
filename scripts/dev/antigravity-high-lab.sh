#!/usr/bin/env bash
# One subscription High acceptance, independent of all older SEALED windows.
set -euo pipefail
root=${1:?agent root required}
phase=${2:-tests}
[[ "$root" == /srv/storage/butler/agents/antigravity-high ]] || exit 2
[[ "$phase" == tests || "$phase" == execute || "$phase" == inspect || "$phase" == probe ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
umask 077
mkdir -p "$root/.cache/docker" "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/reports" "$root/runtime"
export DOCKER_CONFIG="$root/.cache/docker"
common=(--runtime runc --pull never --read-only --cap-drop ALL --security-opt no-new-privileges \
 --memory 2g --pids-limit 128 --cpus 2 --tmpfs /tmp:rw,nosuid,nodev,size=256m)
build=(--user "$(id -u):$(id -g)" --env GOCACHE=/cache/go-build --env GOMODCACHE=/modules --env GOTMPDIR=/cache/go-tmp \
 --mount "type=bind,source=$src,target=/src,readonly" --mount "type=bind,source=$root/.cache,target=/cache" \
 --mount type=bind,source=/srv/storage/butler/.cache/go-mod,target=/modules,readonly --workdir /src)
if [[ "$phase" == execute ]];then
 [[ ${BUTLER_HIGH_APPROVED:-} == 1 && "$(cat "$root/runtime/tested-source")" == "$src" ]] || exit 2
 mkdir "$root/window-20261012-high-22-fresh" || { printf '{"status":"high_window_sealed"}\n';exit 2; }
 # The atomic directory never resets; preparation failures also seal this window.
 printf '%s\n' "$src" > "$root/window-20261012-high-22-fresh/source"
fi
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/high-$phase-$run_id"
mkdir -p "$proof/postgres"
name="butler-agy-high-$run_id"
network="$name-net"
pg=false;net=false;runner=false
cleanup(){
 $runner && docker rm -f "$name-run" >/dev/null 2>&1 || true
 if $pg;then
  docker exec "$name-pg" pg_dump -U fixture -d comparison > "$proof/ledger.sql" 2>/dev/null || true
  docker rm -f "$name-pg" >/dev/null 2>&1 || true
 fi
 $net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal "$network" >/dev/null;net=true
docker run -d --name "$name-pg" --runtime runc --pull never --network "$network" --network-alias compare-postgres \
 --memory 256m --pids-limit 64 --security-opt no-new-privileges \
 --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=compare-fixture --env POSTGRES_DB=comparison \
 --mount "type=bind,source=$proof/postgres,target=/var/lib/postgresql/data" postgres:17 \
 -c shared_buffers=16MB -c max_connections=16 >/dev/null;pg=true
for i in $(seq 1 30);do docker exec "$name-pg" pg_isready -U fixture -d comparison >/dev/null 2>&1 && break;[[ "$i" != 30 ]] || exit 1;sleep 1;done
if [[ "$phase" == tests ]];then
 docker run --rm "${common[@]}" "${build[@]}" --network "$network" \
  --mount type=bind,source=/srv/storage/butler/native-acceptance-windows/baseline13,target=/baseline13,readonly \
  --env 'BUTLER_TEST_COMPARE_POSTGRES_URL=postgres://fixture:compare-fixture@compare-postgres:5432/comparison?sslmode=disable' \
  --env 'BUTLER_TEST_PROVIDER_AUTH_POSTGRES_URL=postgres://fixture:compare-fixture@compare-postgres:5432/comparison?sslmode=disable' \
  golang:1.27.2-bookworm go test -json -race -p 1 -count=1 ./scripts/dev/antigravity-compare ./internal/transport/antigravity ./internal/providerauth > "$proof/tests.jsonl"
 docker run --rm "${common[@]}" "${build[@]}" --network none golang:1.27.2-bookworm \
  go vet ./scripts/dev/antigravity-compare ./internal/transport/antigravity ./internal/providerauth
 printf '%s\n' "$src" > "$root/runtime/tested-source"
 sha256sum "$proof/tests.jsonl"
 printf 'Zero-inference High tests: %s\n' "$proof"
 exit 0
fi
docker run --rm "${common[@]}" "${build[@]}" --network none --env CGO_ENABLED=0 \
 --mount "type=bind,source=$root/runtime,target=/out" golang:1.27.2-bookworm \
 go build -o /out/antigravity-high ./scripts/dev/antigravity-compare
auth_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
mode=high
[[ "$phase" != inspect ]] || mode=high-inspect
[[ "$phase" != probe ]] || mode=-probe-network
docker create "${common[@]}" --name "$name-run" --user 10001:10001 --network "$network" \
 --env BUTLER_HIGH_APPROVED=1 \
 --env 'BUTLER_COMPARE_POSTGRES_URL=postgres://fixture:compare-fixture@compare-postgres:5432/comparison?sslmode=disable' \
 --mount "type=bind,source=$src,target=/src,readonly" \
 --mount "type=bind,source=$root/runtime/antigravity-high,target=/high,readonly" \
 --mount type=bind,source=/srv/storage/butler/agents/antigravity-compare/private/control.env,target=/private/control.env,readonly \
 --mount "type=bind,source=$auth_parent/butler-native-import.json,target=/auth/import.json,readonly" \
 --mount type=bind,source=/srv/storage/butler/native-acceptance-windows/baseline13,target=/baseline13,readonly \
 golang:1.27.2-bookworm /high "$mode" >/dev/null;runner=true
docker network connect butler-lab_backend "$name-run"
docker start -a "$name-run" > "$proof/report.json"
[[ "$(docker inspect --format '{{.State.ExitCode}}' "$name-run")" == 0 ]] || exit 1
sha256sum "$proof/report.json"
cat "$proof/report.json"
printf 'High acceptance proof: %s\n' "$proof"

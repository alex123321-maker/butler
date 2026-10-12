#!/usr/bin/env bash
# Separate private comparison. Never reopens a native acceptance window.
set -euo pipefail
root=${1:?isolated comparison root required}
[[ "$root" == /srv/storage/butler/agents/antigravity-compare ]] || exit 2
phase=${2:-tests}
[[ "$phase" == tests || "$phase" == inspect || "$phase" == execute || "$phase" == native8192 ]] || exit 2
catalog_only=${BUTLER_COMPARE_CATALOG_ONLY:-0}
[[ "$catalog_only" == 0 || "$catalog_only" == 1 ]] || exit 2
[[ "$phase" != execute || "$catalog_only" == 0 ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
export DOCKER_CONFIG="$root/.cache/docker"
umask 077
mkdir -p "$root/.cache/docker" "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/runtime" "$root/reports"
common=(--runtime runc --pull never --read-only --cap-drop ALL --security-opt no-new-privileges
 --memory 2g --pids-limit 128 --cpus 2 --tmpfs /tmp:rw,nosuid,nodev,size=256m)
build=(--user "$(id -u):$(id -g)" --env GOCACHE=/cache/go-build --env GOMODCACHE=/modules --env GOTMPDIR=/cache/go-tmp
 --mount "type=bind,source=$source_dir,target=/src,readonly"
 --mount "type=bind,source=$root/.cache,target=/cache"
 --mount "type=bind,source=/srv/storage/butler/.cache/go-mod,target=/modules,readonly" --workdir /src)
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/compare-$phase-$run_id"
mkdir -p "$proof/postgres"
name="butler-agy-compare-$run_id"
network="$name-net"
created_net=false;created_pg=false;created_gate=false;created_cli=false
cleanup(){
 $created_cli && docker rm -f "$name-cli" >/dev/null 2>&1 || true
 if $created_pg; then
  docker exec "$name-pg" pg_dump -U fixture -d comparison > "$proof/ledger.sql" 2>/dev/null || true
 fi
 if $created_gate; then
  docker logs "$name-gate" > "$proof/backend-safe.jsonl" 2>/dev/null || true
  docker rm -f "$name-gate" >/dev/null 2>&1 || true
 fi
 $created_pg && docker rm -f "$name-pg" >/dev/null 2>&1 || true
 $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
if [[ "$phase" == execute || "$phase" == native8192 ]]; then
 [[ ${BUTLER_COMPARE_APPROVED:-} == 1 ]] || exit 2
 # This is a NEW, single-use diagnostic window. The five older windows stay sealed.
 window=window-20261011-two-client
 [[ "$phase" != native8192 ]] || window=window-20261011-native8192
 mkdir "$root/$window" || { printf '{"status":"comparison_window_sealed"}\n'; exit 2; }
fi
docker run --rm "${common[@]}" "${build[@]}" --network none golang:1.27.2-bookworm \
 go vet ./scripts/dev/antigravity-compare
docker network create --internal "$network" >/dev/null;created_net=true
docker run -d --name "$name-pg" --runtime runc --pull never --network "$network" --network-alias compare-postgres \
 --memory 256m --pids-limit 64 --security-opt no-new-privileges \
 --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=compare-fixture --env POSTGRES_DB=comparison \
 --mount "type=bind,source=$proof/postgres,target=/var/lib/postgresql/data" postgres:17 \
 -c shared_buffers=16MB -c max_connections=16 >/dev/null;created_pg=true
for i in $(seq 1 30);do docker exec "$name-pg" pg_isready -U fixture -d comparison >/dev/null 2>&1 && break;[[ "$i" != 30 ]] || exit 1;sleep 1;done
if [[ "$phase" == tests ]];then
 docker run --rm "${common[@]}" "${build[@]}" --network "$network" \
  --mount type=bind,source=/srv/storage/butler/native-acceptance-windows/baseline13,target=/baseline13,readonly \
  --env 'BUTLER_TEST_COMPARE_POSTGRES_URL=postgres://fixture:compare-fixture@compare-postgres:5432/comparison?sslmode=disable' \
  golang:1.27.2-bookworm go test -json -race -p 1 -count=1 ./scripts/dev/antigravity-compare > "$proof/tests.jsonl"
 sha256sum "$proof/tests.jsonl"
 printf 'Zero-inference gate tests: %s\n' "$proof"
 exit 0
fi
cli=/srv/storage/butler/agents/antigravity-provider/runtime/agy
[[ "$(sha256sum "$cli"|cut -d ' ' -f 1)" == d97edcf90eda1799cd6cedc92616ef402f23c9752a723e8b8ae3e8f062ce458b ]] || exit 2
docker run --rm "${common[@]}" "${build[@]}" --network none --env CGO_ENABLED=0 \
 --mount "type=bind,source=$root/runtime,target=/out" golang:1.27.2-bookworm \
 go build -o /out/antigravity-compare ./scripts/dev/antigravity-compare
if [[ ! -e "$root/private-bootstrap.done" ]];then
 mkdir -p "$root/private"
 node "$source_dir/scripts/dev/antigravity-compare/private-env.mjs" "$root/private/control.env"
 docker run --rm --network none --pull never --mount "type=bind,source=$root/private,target=/private" alpine:3.22 \
  sh -eu -c 'chown -R 10001:10001 /private; chmod 700 /private; chmod 600 /private/control.env'
 touch "$root/private-bootstrap.done"
fi
auth_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
docker create "${common[@]}" --name "$name-gate" --user 10001:10001 --network "$network" --network-alias compare-gate \
 --env "BUTLER_COMPARE_APPROVED=${BUTLER_COMPARE_APPROVED:-0}" \
 --env 'BUTLER_COMPARE_POSTGRES_URL=postgres://fixture:compare-fixture@compare-postgres:5432/comparison?sslmode=disable' \
 --mount "type=bind,source=$source_dir,target=/src,readonly" \
 --mount "type=bind,source=$root/runtime/antigravity-compare,target=/compare,readonly" \
 --mount "type=bind,source=$cli,target=/installed-cli,readonly" \
 --mount "type=bind,source=$root/private,target=/private" \
 --mount "type=bind,source=$auth_parent/butler-native-import.json,target=/auth/import.json,readonly" \
 --mount "type=bind,source=$auth_parent/antigravity-oauth-token,target=/auth/cli.json,readonly" \
 --mount type=bind,source=/srv/storage/butler/native-acceptance-windows/baseline13,target=/baseline13,readonly \
 golang:1.27.2-bookworm /compare "$phase" >/dev/null;created_gate=true
docker network connect butler-lab_backend "$name-gate"
docker start "$name-gate" >/dev/null
ready=false
for i in $(seq 1 100);do
 if docker exec "$name-gate" /compare -control ready > "$proof/ready.json" 2>/dev/null;then ready=true;break;fi
 [[ "$(docker inspect --format '{{.State.Running}}' "$name-gate")" == true ]] || break
 sleep 1
done
$ready || { printf '{"status":"private_preflight_failed","new_model_calls":0}\n';exit 1; }
if [[ "$phase" == native8192 ]];then
 docker exec "$name-gate" /compare -control run-go > "$proof/report.json"
 docker exec "$name-gate" /compare -control post-metadata > "$proof/report.json"
 docker exec "$name-gate" /compare -control shutdown > "$proof/report.json"
 sha256sum "$proof/report.json"
 cat "$proof/report.json"
 printf 'Safe native8192 evidence: %s\n' "$proof"
 exit 0
fi
docker run -d "${common[@]}" --name "$name-cli" --log-driver none --user 10001:10001 --network "$network" \
 --env HOME=/private/home --env XDG_CONFIG_HOME=/private/home --env ANTIGRAVITY_APP_DATA_DIR=/private/home/.gemini/antigravity-cli \
 --env HTTPS_PROXY=http://compare-gate:3128 --env HTTP_PROXY=http://compare-gate:3128 --env SSL_CERT_FILE=/private/ca.pem --env TERM=dumb \
 --env NO_PROXY=localhost,127.0.0.1,0.0.0.0,::1,:: \
 --env "COMPARE_CATALOG_ONLY=$catalog_only" \
 --mount "type=bind,source=$root/private/home,target=/private/home" \
 --mount "type=bind,source=$root/private/work,target=/private/work" \
 --mount "type=bind,source=$root/private/cli-logs,target=/private/cli-logs" \
 --mount "type=bind,source=$root/private/ca.pem,target=/private/ca.pem,readonly" \
 --mount "type=bind,source=$root/private/model-arg,target=/private/model-arg,readonly" \
 --mount "type=bind,source=$root/private/prompt,target=/private/prompt,readonly" \
 --mount "type=bind,source=$cli,target=/agy,readonly" --workdir /private/work --entrypoint /bin/sh golang:1.27.2-bookworm \
 -eu -c 'if [ "$COMPARE_CATALOG_ONLY" = 1 ];then /agy models > /private/cli-logs/output 2>&1;else /agy --agent compare --model "$(cat /private/model-arg)" --print "$(cat /private/prompt)" --print-timeout 40s --output-format json --disable-slash-commands > /private/cli-logs/output 2>&1;fi' >/dev/null;created_cli=true
for i in $(seq 1 48);do
 docker exec "$name-gate" /compare -control report > "$proof/cli-observation.json" 2>/dev/null || break
 if [[ "$phase" == inspect ]] && grep -q '"generation_seen":true' "$proof/cli-observation.json";then break;fi
 [[ "$(docker inspect --format '{{.State.Running}}' "$name-cli")" == true ]] || break
 sleep 1
done
cli_running=$(docker inspect --format '{{.State.Running}}' "$name-cli")
cli_exit=$(docker inspect --format '{{.State.ExitCode}}' "$name-cli")
printf '{"running_when_stopped":%s,"exit_code":%s}\n' "$cli_running" "$cli_exit" > "$proof/cli-exit.json"
docker rm -f "$name-cli" >/dev/null;created_cli=false
docker exec "$name-gate" /compare -control report > "$proof/report.json"
if [[ "$phase" == execute ]];then
 # Native comparison is a distinct admitted call, never a retry of CLI failure.
 docker exec "$name-gate" /compare -control run-go > "$proof/report.json"
 docker exec "$name-gate" /compare -control post-metadata > "$proof/report.json"
fi
docker exec "$name-gate" /compare -control shutdown > "$proof/report.json"
sha256sum "$proof/report.json"
cat "$proof/report.json"
printf 'Safe comparison evidence: %s\n' "$proof"

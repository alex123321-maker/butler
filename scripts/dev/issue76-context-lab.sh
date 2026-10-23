#!/usr/bin/env bash
# Bounded paired observation; no production changes or historical-window reuse.
set -euo pipefail
source_root=${1:?isolated source root}
root=${2:?fresh trial root}
action=${3:?prepare or execute}
release=${4:?exact source commit}
[[ "$source_root" =~ ^/srv/storage/butler/agents/issue76-source-[a-zA-Z0-9_-]+$ && "$root" =~ ^/srv/storage/butler/agents/issue76-trial-[a-zA-Z0-9_-]+$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
umask 077
if [[ "$action" == prepare ]]; then
 [[ ! -e "$root" ]] || { echo 'Trial already exists'; exit 2; }
 src=$(readlink -f "$source_root/current")
 [[ "$src" == "$source_root/snapshots/"* ]] || exit 2
 mkdir -p "$root/build" "$root/metadata" "$root/private" "$root/postgres" "$root/cache/go-build" "$root/cache/go-tmp"
 image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
 cache="$root/cache"
 docker run --rm --network none --cpus 2 --memory 1536m --pids-limit 128 \
  -v "$src:/src:ro" -v "$cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$root/build:/build" -w /src \
  -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod "$image" \
  go test -c -p 1 -o /build/collector.test ./apps/orchestrator/internal/app > "$root/build.log" 2> "$root/build-diagnostics.log"
 docker run --rm --user "$(id -u):$(id -g)" --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  -v "$src:/src:ro" -v "$root/build:/runtime:ro" -v "$root/metadata:/metadata" -w /src \
  -e BUTLER_ISSUE76_FREEZE_DIR=/metadata -e BUTLER_MEMORY_EVAL_RELEASE="$release" "$image" \
  /runtime/collector.test -test.run '^TestIssue76(Freeze|SchemaWithoutCredentials)$' -test.v > "$root/freeze.log" 2>&1
 jq --arg image "$image" --arg trial "$(basename "$root")" --arg src "$src" --arg root "$root" \
  '.+{Image:$image,TrialID:$trial,SourceDir:$src,TrialRoot:$root}' "$root/metadata/pins.json" > "$root/metadata/pins.tmp"
 mv "$root/metadata/pins.tmp" "$root/metadata/pins.json"
 docker run --rm --network none --read-only --cap-drop ALL --cap-add CHOWN --security-opt no-new-privileges:true \
  -v "$root/private:/private" "$image" chown 10001:10001 /private
 chmod 711 "$root" "$root/build" "$root/metadata"
 chmod 755 "$root/build/collector.test"
 chmod 644 "$root/metadata/"*.json
 printf 'Prepared, zero native calls: %s\n' "$root"
 exit 0
fi
[[ "$action" == execute && -f "$root/metadata/pins.json" && -f "$root/metadata/review.json" && ! -e "$root/entered" ]] || exit 2
jq -e --slurpfile pins "$root/metadata/pins.json" '.Status=="approved_for_native_diagnostic" and ([.Release,.Source,.Binary,.Frozen,.Protocol,.Controls,.Development]==[$pins[0].Release,$pins[0].Source,$pins[0].Binary,$pins[0].Frozen,$pins[0].Protocol,$pins[0].Controls,$pins[0].Development])' "$root/metadata/review.json" >/dev/null
[[ "$(jq -r .Release "$root/metadata/pins.json")" == "$release" ]] || exit 2
src=$(jq -r .SourceDir "$root/metadata/pins.json")
image=$(jq -r .Image "$root/metadata/pins.json")
[[ "$src" == "$source_root/snapshots/"* && "$image" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
tag=$(basename "$root")
native_import=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli/butler-native-import.json
mkdir -m 700 "$root/entered"
docker network create --internal --subnet 10.254.76.0/28 "$tag" > "$root/entered/network-id"
docker run -d --name "$tag-pg" --network "$tag" --network-alias memory-eval-postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$root/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=issue76_test \
 pgvector/pgvector:pg17 > "$root/entered/postgres-id"
finish() {
 docker stop --time 15 "$tag-native" >/dev/null 2>&1 || true
 docker exec "$tag-pg" psql -U butler -d issue76_test -v ON_ERROR_STOP=1 -c 'UPDATE fixture_memory_eval_window SET closed=TRUE WHERE NOT closed' > "$root/entered/external-seal.log" 2> "$root/entered/external-seal-diagnostics.log" || true
 docker stop "$tag-pg" >/dev/null || true
}
trap finish EXIT
ready=false
for _ in $(seq 1 30); do if docker exec "$tag-pg" pg_isready -U butler -d issue76_test >/dev/null; then ready=true; break; fi; sleep 1; done
[[ "$ready" == true ]] || exit 2
docker create --name "$tag-native" --user 10001:10001 --network "$tag" --read-only --cap-drop ALL --security-opt no-new-privileges:true \
 --cpus 2 --memory 1536m --pids-limit 128 --tmpfs /tmp:rw,noexec,nosuid,size=16777216 \
 -v "$src:/src:ro" -v "$root/build:/runtime:ro" -v "$root/metadata:/metadata:ro" -v "$root/private:/private" \
 --mount "type=bind,src=$native_import,dst=/auth/native-import.json,readonly" \
 -e HTTPS_PROXY=http://antigravity-proxy:18080 -e HTTP_PROXY=http://antigravity-proxy:18080 -e NO_PROXY=localhost,127.0.0.1,memory-eval-postgres \
 -w /src -e BUTLER_ISSUE76_NATIVE=1 -e BUTLER_MEMORY_EVAL_PRIVATE=/private -e BUTLER_MEMORY_EVAL_RELEASE="$release" \
 -e BUTLER_MEMORY_EVAL_IMPORT_FILE=/auth/native-import.json -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@memory-eval-postgres:5432/issue76_test?sslmode=disable' \
 "$image" /runtime/collector.test -test.run '^TestIssue76NativeTrial$' -test.v -test.timeout 185m > "$root/entered/native-container-id"
docker network connect butler-lab_backend "$tag-native"
docker inspect "$tag-native" | jq '.[0]|{Id,Image,Path,Args,User:.Config.User,Memory:.HostConfig.Memory,NanoCpus:.HostConfig.NanoCpus,PidsLimit:.HostConfig.PidsLimit,ReadonlyRootfs:.HostConfig.ReadonlyRootfs,Privileged:.HostConfig.Privileged,CapDrop:.HostConfig.CapDrop,CapAdd:.HostConfig.CapAdd,SecurityOpt:.HostConfig.SecurityOpt,Tmpfs:.HostConfig.Tmpfs,ProxyConfigured:((.Config.Env|index("HTTPS_PROXY=http://antigravity-proxy:18080")!=null) and (.Config.Env|index("HTTP_PROXY=http://antigravity-proxy:18080")!=null)),LocalBypass:(.Config.Env|index("NO_PROXY=localhost,127.0.0.1,memory-eval-postgres")!=null),Networks:(.NetworkSettings.Networks|keys),Mounts:[.Mounts[]|{Type,Source,Destination,RW}]}' > "$root/metadata/execution.json"
chmod 644 "$root/metadata/execution.json"
set +e
timeout --foreground 190m docker start -a "$tag-native" > "$root/native.log" 2> "$root/native-diagnostics.log"
status=$?
set -e
printf '%s\n' "$status" > "$root/entered/native-exit-status"
echo "Trial retained: $root; exit=$status"
exit "$status"

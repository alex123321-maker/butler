#!/usr/bin/env bash
# Fresh semantic protocol; it never reads/reopens any historical native root.
set -euo pipefail
source_root=${1:?isolated source root}
root=${2:?fresh semantic trial root}
action=${3:?prepare or execute}
release=${4:?exact clean commit}
[[ "$source_root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ && "$root" =~ ^/srv/storage/butler/agents/memory-semantic-trial-[a-zA-Z0-9_-]+$ && "$release" =~ ^[a-f0-9]{40}$ && "$source_root" != "$root" ]] || exit 2
umask 077
cache=/srv/storage/butler/agents/memory-eval-78-20261014/.cache
if [[ "$action" == prepare ]]; then
 [[ ! -e "$root" ]] || { echo 'Fresh trial root already exists'; exit 2; }
 src=$(readlink -f "$source_root/current")
 [[ "$src" == "$source_root/snapshots/"* ]] || exit 2
 mkdir -p "$root/build" "$root/private" "$root/metadata" "$root/postgres"
 printf '%s\n' "$src" > "$root/source-path"
 image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
 printf '%s\n' "$image" > "$root/image-id"
 printf '%s\n' "$release" > "$root/release"
 docker run --rm --network none --cpus 2 --memory 1536m --pids-limit 128 \
  -v "$src:/src:ro" -v "$cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$root/build:/build" -w /src \
  -e GOCACHE=/cache/go-timeout -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod "$image" bash -eu -c '
go test -c -p 1 -o /build/collector.test ./apps/orchestrator/internal/app
go build -p 1 -o /build/memory-eval ./cmd/memory-eval
' > "$root/build.log" 2> "$root/build-diagnostics.log"
 docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  -v "$src:/src:ro" -v "$root/build:/build:ro" "$image" /build/memory-eval -mode semantic-score -pins-only \
  -root /src -binary /build/collector.test -code-commit "$release" \
  -suite /src/testdata/acceptance/ru-memory-semantic-v1.json -rubric /src/docs/ai/memory-eval-semantic-methodology.md | \
  jq --arg image "$image" --arg trial "$(basename "$root")" --arg src "$src" --arg root "$root" '.+{Image:$image,TrialID:$trial,SourceDir:$src,TrialRoot:$root}' > "$root/metadata/semantic-pins.json"
 sha256sum "$src/docs/ai/memory-eval-semantic-preflight-review-20261015.md" \
  "$src/testdata/acceptance/reviews/ru-memory-semantic-calibration-review-20261015.json" > "$root/metadata/independent-review.sha256"
 docker run --rm --network none --read-only --cap-drop ALL --cap-add CHOWN --security-opt no-new-privileges:true \
  -v "$root/private:/private" "$image" chown 10001:10001 /private
 chmod 711 "$root" "$root/build" "$root/metadata"
 chmod 755 "$root/build/collector.test" "$root/build/memory-eval"
 chmod 644 "$root/metadata/semantic-pins.json"
 printf 'Prepared source/binary/corpus/method pins: %s\n' "$root"
 exit 0
fi
[[ "$action" == execute && -f "$root/metadata/semantic-pins.json" && ! -e "$root/entered" ]] || exit 2
[[ "$(cat "$root/release")" == "$release" ]] || exit 2
src=$(cat "$root/source-path")
[[ "$src" == "$source_root/snapshots/"* ]] || exit 2
sha256sum -c "$root/metadata/independent-review.sha256" > "$root/review-recheck.log"
image=$(cat "$root/image-id")
[[ "$image" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
tag=$(basename "$root")
token=/srv/storage/butler/agents/local-models/private/token
native_import=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli/butler-native-import.json
# The owner authorized these fixed fresh cases. One entry per root prevents a
# failed answer/attempt from being silently replaced by another run.
mkdir -m 700 "$root/entered"
docker network create --internal "$tag" > "$root/entered/network-id"
docker run -d --name "$tag-pg" --network "$tag" --network-alias memory-eval-postgres --cpus 1 --memory 512m --pids-limit 128 \
 -v "$root/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_eval_test \
 pgvector/pgvector:pg17 > "$root/entered/postgres-id"
finish() {
 for phase in write read; do docker stop --time 15 "$tag-$phase" >/dev/null 2>&1 || true; done
 docker exec "$tag-pg" psql -U butler -d memory_eval_test -v ON_ERROR_STOP=1 -c 'UPDATE fixture_memory_eval_window SET closed=TRUE WHERE singleton AND NOT closed' > "$root/entered/external-seal.log" 2> "$root/entered/external-seal-diagnostics.log" || true
 docker stop "$tag-pg" >/dev/null || true
}
trap finish EXIT
ready() { for _ in $(seq 1 30); do if docker exec "$tag-pg" pg_isready -U butler -d memory_eval_test >/dev/null; then return; fi; sleep 1; done; return 1; }
ready
for phase in write read; do
 auth_args=(); proxy_args=()
 if [[ "$phase" == read ]]; then
  auth_args=(--mount "type=bind,src=$native_import,dst=/auth/native-import.json,readonly" -e BUTLER_MEMORY_EVAL_IMPORT_FILE=/auth/native-import.json)
  proxy_args=(-e HTTPS_PROXY=http://antigravity-proxy:18080 -e HTTP_PROXY=http://antigravity-proxy:18080 -e NO_PROXY=localhost,127.0.0.1,memory-eval-postgres,local-models)
 fi
 docker create --name "$tag-$phase" --user 10001:10001 --network "$tag" --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --cpus 2 --memory 1536m --pids-limit 128 --tmpfs /tmp:rw,noexec,nosuid,size=16777216 \
  -v "$src:/src:ro" -v "$root/build:/runtime:ro" -v "$root/metadata:/metadata:ro" -v "$root/private:/private" \
  --mount "type=bind,src=$token,dst=/auth/local-token,readonly" "${auth_args[@]}" "${proxy_args[@]}" \
  -w /src -e BUTLER_MEMORY_SEMANTIC_PHASE="$phase" -e BUTLER_MEMORY_EVAL_PRIVATE=/private -e BUTLER_MEMORY_EVAL_RELEASE="$release" \
  -e BUTLER_MEMORY_EVAL_LOCAL_TOKEN_FILE=/auth/local-token -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@memory-eval-postgres:5432/memory_eval_test?sslmode=disable' \
  "$image" /runtime/collector.test -test.run '^TestMemorySemanticActualTrial$' -test.v -test.timeout 245m > "$root/entered/$phase-container-id"
 docker network connect butler-lab_local-inference "$tag-$phase"
 if [[ "$phase" == read ]]; then docker network connect butler-lab_backend "$tag-$phase"; fi
 docker inspect "$tag-$phase" | jq '.[0]|{Id,Image,Path,Args,User:.Config.User,Memory:.HostConfig.Memory,NanoCpus:.HostConfig.NanoCpus,PidsLimit:.HostConfig.PidsLimit,ReadonlyRootfs:.HostConfig.ReadonlyRootfs,Privileged:.HostConfig.Privileged,CapDrop:.HostConfig.CapDrop,CapAdd:.HostConfig.CapAdd,SecurityOpt:.HostConfig.SecurityOpt,Tmpfs:.HostConfig.Tmpfs,ProxyConfigured:((.Config.Env|index("HTTPS_PROXY=http://antigravity-proxy:18080")!=null) and (.Config.Env|index("HTTP_PROXY=http://antigravity-proxy:18080")!=null)),LocalBypass:(.Config.Env|index("NO_PROXY=localhost,127.0.0.1,memory-eval-postgres,local-models")!=null),Networks:(.NetworkSettings.Networks|keys),Mounts:[.Mounts[]|{Type,Source,Destination,RW}]}' > "$root/metadata/$phase-execution.json"
 chmod 644 "$root/metadata/$phase-execution.json"
 set +e
 timeout --foreground 250m docker start -a "$tag-$phase" > "$root/$phase.log" 2> "$root/$phase-diagnostics.log"
 status=$?
 set -e
 printf '%s\n' "$status" > "$root/entered/$phase-exit-status"
 [[ "$status" == 0 ]] || { echo "Retained failed $phase prefix: $root"; exit "$status"; }
 if [[ "$phase" == write ]]; then
  docker restart "$tag-pg" > "$root/entered/database-restart-id"
  ready
 fi
done
printf 'Fresh semantic collection complete: %s\n' "$root"

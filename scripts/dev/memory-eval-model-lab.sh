#!/usr/bin/env bash
# Prepare once, execute once, then read-only audit. Never reopens old windows.
set -euo pipefail
source_root=${1:?isolated synchronized source root}
root=${2:?new model experiment root}
action=${3:?prepare execute or audit}
release=${4:?exact candidate commit}
prior_root=${5:-}
[[ "$source_root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ && "$root" =~ ^/srv/storage/butler/agents/memory-eval-v2-[a-zA-Z0-9_-]+$ && "$release" =~ ^[a-f0-9]{40}$ && "$root" != "$source_root" ]] || exit 2
[[ -z "$prior_root" || ( "$prior_root" == /srv/storage/butler/agents/memory-eval-v2-native-20261015 || "$prior_root" == /srv/storage/butler/agents/memory-eval-v2-pending-20261015 || "$prior_root" == /srv/storage/butler/agents/memory-eval-v2-residual-20261015 ) && "$root" != "$prior_root" ]] || exit 2
umask 077
# SSH operator UID need not own the confined runtime UID's files. Helpers have
# no network, no credential mount, and only the new experiment's named outputs.
own_metadata() {
 [[ "$1" == pins.json || "$1" == execution.json || "$1" == prior.json ]] || exit 2
 docker run --rm --network none --read-only --cap-drop ALL --cap-add CHOWN --security-opt no-new-privileges:true \
  --mount "type=bind,src=$root/metadata,dst=/metadata" "$image_id" chown 10001:10001 "/metadata/$1"
}
read_output() {
 case "$1" in metadata/hardware.json|metadata/pins.json|build/pin-fixture/frozen.json|private/frozen.json|audit/report/report.json) ;; *) exit 2;; esac
 docker run --rm --user 10001:10001 --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --mount "type=bind,src=$root/$1,dst=/output,readonly" "$image_id" cat /output
}
hash_output() {
 case "$1" in build/collector.test|build/memory-eval|metadata/pins.json|review/approval.json|review/public-key.hex) ;; *) exit 2;; esac
 docker run --rm --user 10001:10001 --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --mount "type=bind,src=$root/$1,dst=/output,readonly" "$image_id" sha256sum /output | cut -d' ' -f1
}
if [[ "$action" == prepare ]]; then
 [[ ! -e "$root" ]] || { echo 'Existing experiment root denied'; exit 2; }
 mkdir -m 700 "$root"
 source_dir=$(readlink -f "$source_root/current")
 [[ "$source_dir" == "$source_root/snapshots/"* ]] || exit 2
 printf '%s\n' "$source_dir" > "$root/source-path"
 mkdir -p "$root/build" "$root/metadata" "$root/review" "$root/private" "$root/postgres" "$root/prior" "$root/.cache/go-build" "$root/.cache/go-tmp" "$root/.cache/docker"
 export DOCKER_CONFIG="$root/.cache/docker"
 image_id=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
 printf '%s\n' "$image_id" > "$root/image-id"
 pg_id=$(docker image inspect pgvector/pgvector:pg17 --format '{{.Id}}')
 jq -n --arg release "$release" --arg kernel "$(uname -r)" --arg image "$image_id" --arg pg "$pg_id" \
  --argjson cpus "$(getconf _NPROCESSORS_ONLN)" --argjson memory "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" \
  '{release:$release,kernel:$kernel,go_image:$image,postgres_image:$pg,host_cpu_count:$cpus,host_memory_kib:$memory,runner:{cpus:2,memory_bytes:1610612736,pids:128,uid:10001,source:"read_only",window_seconds:10800},worker:"existing single authenticated local-models:9090",native_model:"gemini-3.8-flash-high",observed_version_required:"gemini-3.8-flash-n",maximum_calls:280,maximum_conservative_units:4000000,maximum_new_native_calls:66,maximum_events_per_entry:4096,maximum_report_bytes:134217728,retries:0,fallback:false,tokenizer:"cloud byte upper bound; local actual token count unavailable",invoice:"subscription unknown; local no billing",ordering:"case order then no_ltm/full_context/retrieval; fixed not randomized"}' > "$root/metadata/hardware.json"
 docker run --rm --network none --cpus 2 --memory 1536m --pids-limit 128 -v "$source_dir:/src:ro" -v "$root/.cache:/cache" \
  -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$root/build:/build" -w /src \
  -e GOCACHE=/cache/go-build -e GOMODCACHE=/gomod -e GOTMPDIR=/cache/go-tmp "$image_id" bash -eu -c '
go test -c -p 1 -o /build/collector.test ./apps/orchestrator/internal/app
go build -p 1 -o /build/memory-eval ./cmd/memory-eval
go build -p 1 -o /build/review-seal ./scripts/dev/memory-eval-review
go build -p 1 -o /build/prior-pin ./scripts/dev/memory-eval-prior
' > "$root/build.log" 2> "$root/build-diagnostics.log"
 docker run --rm --network none --read-only --cap-drop ALL --cap-add CHOWN --cap-add FOWNER --cap-add DAC_OVERRIDE --security-opt no-new-privileges:true \
  --mount "type=bind,src=$root/build,dst=/build" --mount "type=bind,src=$root/private,dst=/private" \
  --mount "type=bind,src=$root/metadata,dst=/metadata" --mount "type=bind,src=$root/review,dst=/review" --mount "type=bind,src=$root/prior,dst=/prior" "$image_id" sh -eu -c '
chown -R 10001:10001 /build /private /review /prior
chown 10001:10001 /metadata/hardware.json
chmod 700 /build/collector.test /build/memory-eval /build/review-seal /build/prior-pin /private /review /prior
chmod 711 /metadata
'
 # Ancestor is traversable to the confined UID; no private file becomes public.
 chmod 711 "$root"
 docker run --rm --user 10001:10001 --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true --cpus 2 --memory 1536m --pids-limit 128 \
  -v "$source_dir:/src:ro" -v "$root/build:/build" -v "$root/metadata:/metadata:ro" -w /src "$image_id" \
  /build/memory-eval -mode fixture -suite testdata/acceptance/ru-memory-v2.json -root /src -hardware /metadata/hardware.json -code-commit "$release" -window-id pre-native-fixture -output-dir /build/pin-fixture > "$root/pin-fixture.log"
 read_output build/pin-fixture/frozen.json | jq --arg bin "$(hash_output build/collector.test)" --arg window "$(basename "$root")" \
  '.plan|{proxy_required:true,window_id:$window,release:.code_commit,source_sha256:.source_sha256,binary_sha256:$bin,hardware_sha256:.hardware_sha256,suite_sha256:.suite_sha256,criteria_sha256:.criteria_sha256,maximum_calls:280,maximum_conservative_units:4000000,cpus:2,memory_bytes:1610612736,pids:128,window_seconds:10800,maximum_events_per_entry:4096,maximum_report_bytes:134217728}' > "$root/metadata/pins.json"
 own_metadata pins.json
 if [[ -n "$prior_root" ]]; then
  [[ -f "$prior_root/entered/exit-status" && "$(cat "$prior_root/entered/exit-status")" != 0 ]] || exit 2
  prior_source=$(cat "$prior_root/source-path")
  [[ "$prior_source" == "$source_root/snapshots/"* ]] || exit 2
  old_prior_mount=()
  if [[ "$prior_root" != /srv/storage/butler/agents/memory-eval-v2-native-20261015 ]]; then old_prior_mount=(--mount "type=bind,src=$prior_root/prior,dst=/old-prior,readonly"); fi
  docker run --rm --user 10001:10001 --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true --cpus 2 --memory 1536m --pids-limit 128 \
   --mount "type=bind,src=$source_dir,dst=/src,readonly" --mount "type=bind,src=$root/build,dst=/build,readonly" \
   --mount "type=bind,src=$prior_root/private,dst=/old,readonly" --mount "type=bind,src=$prior_root/metadata,dst=/old-meta,readonly" \
   --mount "type=bind,src=$prior_root/build,dst=/old-bin,readonly" --mount "type=bind,src=$prior_source,dst=/old-src,readonly" \
   --mount "type=bind,src=$root/prior,dst=/snapshot" "${old_prior_mount[@]}" "$image_id" /build/prior-pin > "$root/metadata/prior.json"
  # pins.json still belongs to UID10001; replace exclusively with the augmented
  # operator metadata before prepared.sha256 is sealed.
  read_output metadata/pins.json | jq --slurpfile prior "$root/metadata/prior.json" '.+{prior:$prior[0]}' > "$root/metadata/pins-augmented.json"
  mv "$root/metadata/pins-augmented.json" "$root/metadata/pins.json"
  own_metadata pins.json
  own_metadata prior.json
 fi
 docker run --rm --user 10001:10001 --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  -v "$source_dir:/src:ro" -v "$root/build:/build:ro" -v "$root/review:/review" "$image_id" \
  /build/review-seal -declaration-sha256 b533980ec505bc0797092d86945dd68032d39fb04a71efb9bb054e9adbcb9337 \
  -document-sha256 39ff6b07131243044f7e5bc52433ac98489e5e72f2182614dd01a4bccbb3d153 > "$root/review-seal.log"
 for file in build/collector.test build/memory-eval metadata/pins.json review/approval.json review/public-key.hex; do
  printf '%s  %s\n' "$(hash_output "$file")" "$root/$file"
 done > "$root/prepared.sha256"
 printf 'Prepared fresh bounded window: %s\n' "$root"
 exit 0
fi
[[ -f "$root/prepared.sha256" ]] || exit 2
image_id=$(cat "$root/image-id")
[[ "$image_id" =~ ^sha256:[a-f0-9]{64}$ ]] || exit 2
while read -r digest path; do
 [[ "$path" == "$root/"* && "$digest" == "$(hash_output "${path#"$root/"}")" ]] || exit 2
done < "$root/prepared.sha256"
source_dir=$(cat "$root/source-path")
[[ "$source_dir" == "$source_root/snapshots/"* && "$(read_output metadata/pins.json | jq -r .release)" == "$release" ]] || exit 2
export DOCKER_CONFIG="$root/.cache/docker"
[[ "$image_id" == "$(read_output metadata/hardware.json | jq -r .go_image)" ]] || exit 2
pg_id=$(read_output metadata/hardware.json | jq -r .postgres_image)
tag=$(basename "$root")
prior_mount=()
if read_output metadata/pins.json | jq -e '.prior!=null' >/dev/null; then prior_mount=(--mount "type=bind,src=$root/prior,dst=/prior,readonly"); fi
if [[ "$action" == execute ]]; then
 [[ ! -e "$root/entered" ]] || { echo 'One-use model window already entered; preserve it'; exit 2; }
 [[ $(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1) == true ]] || exit 2
 token=/srv/storage/butler/agents/local-models/private/token
 native_import=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli/butler-native-import.json
 # Metadata-only readability check as the actual UID, before the one-use entry.
 # --mount refuses a missing host file instead of creating a directory for it.
 docker run --rm --user 10001:10001 --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --mount "type=bind,src=$token,dst=/auth/local-token,readonly" \
  --mount "type=bind,src=$native_import,dst=/auth/native-import.json,readonly" "$image_id" sh -eu -c \
  'test -f /auth/local-token && test -r /auth/local-token && test -f /auth/native-import.json && test -r /auth/native-import.json'
 # Native auth/refresh happens only after entry in the actual runtime's
 # encrypted PostgreSQL SecretStore; never in a disposable metadata process.
 # Authenticate only Describe on the actual local-inference network. This
 # compiled entry forbids Infer and has neither native import nor DB mounts.
 docker run --rm --user 10001:10001 --network butler-lab_local-inference --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --cpus 2 --memory 1536m --pids-limit 128 --mount "type=bind,src=$root/build,dst=/runtime,readonly" \
  --mount "type=bind,src=$token,dst=/auth/local-token,readonly" \
  -e BUTLER_MEMORY_EVAL_LOCAL_PREFLIGHT=1 -e BUTLER_MEMORY_EVAL_LOCAL_TOKEN_FILE=/auth/local-token \
  "$image_id" /runtime/collector.test -test.run '^TestMemoryEvalLocalMetadataPreflight$' -test.v -test.timeout 20s > "$root/local-metadata-preflight.log"
 # Atomic durable entry marker precedes all new native/auxiliary work.
 mkdir -m 700 "$root/entered" || { echo 'One-use model window already entered; preserve it'; exit 2; }
 docker network create --internal "$tag" > "$root/entered/network-id"
 docker run -d --name "$tag-pg" --network "$tag" --network-alias memory-eval-postgres --cpus 1 --memory 512m --pids-limit 128 \
  --security-opt no-new-privileges:true -v "$root/postgres:/var/lib/postgresql/data" \
  -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_eval_test "$pg_id" > "$root/entered/postgres-id"
 for _ in $(seq 1 30); do if docker exec "$tag-pg" pg_isready -U butler -d memory_eval_test >/dev/null; then break; fi; sleep 1; done
 docker create --name "$tag-runner" --user 10001:10001 --network "$tag" --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --cpus 2 --memory 1536m --pids-limit 128 --tmpfs /tmp:rw,noexec,nosuid,size=67108864 \
  -v "$source_dir:/src:ro" -v "$root/build:/runtime:ro" -v "$root/metadata:/metadata:ro" -v "$root/review:/review:ro" -v "$root/private:/private" \
  "${prior_mount[@]}" \
  -v "$token:/auth/local-token:ro" -v "$native_import:/auth/native-import.json:ro" -w /src \
  -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgresql://butler@memory-eval-postgres:5432/memory_eval_test?sslmode=disable' \
  -e BUTLER_MEMORY_EVAL_ACTUAL=1 -e BUTLER_MEMORY_EVAL_RELEASE="$release" -e BUTLER_MEMORY_EVAL_PRIVATE=/private \
  -e BUTLER_MEMORY_EVAL_LOCAL_TOKEN_FILE=/auth/local-token -e BUTLER_MEMORY_EVAL_IMPORT_FILE=/auth/native-import.json \
  -e HTTPS_PROXY=http://antigravity-proxy:18080 -e HTTP_PROXY=http://antigravity-proxy:18080 -e NO_PROXY=localhost,127.0.0.1,memory-eval-postgres,local-models \
  "$image_id" /runtime/collector.test -test.run '^TestMemoryEvalActualCollection$' -test.v -test.timeout 185m > "$root/entered/runner-id"
 docker network connect butler-lab_backend "$tag-runner"
 docker network connect butler-lab_local-inference "$tag-runner"
 docker inspect "$tag-runner" | jq '.[0]|{Id,Image,Path,Args,User:.Config.User,Memory:.HostConfig.Memory,NanoCpus:.HostConfig.NanoCpus,PidsLimit:.HostConfig.PidsLimit,ReadonlyRootfs:.HostConfig.ReadonlyRootfs,ProxyConfigured:((.Config.Env|index("HTTPS_PROXY=http://antigravity-proxy:18080")!=null) and (.Config.Env|index("HTTP_PROXY=http://antigravity-proxy:18080")!=null)),LocalBypass:(.Config.Env|index("NO_PROXY=localhost,127.0.0.1,memory-eval-postgres,local-models")!=null),Networks:(.NetworkSettings.Networks|keys),Mounts:[.Mounts[]|{Destination,RW}]}' > "$root/metadata/execution.json"
 own_metadata execution.json
 set +e
 timeout 11100 docker start -a "$tag-runner" > "$root/runner.log" 2> "$root/runner-diagnostics.log"
 status=$?
 set -e
 if [[ "$status" != 0 ]]; then docker stop --time 10 "$tag-runner" >/dev/null || true; fi
 printf '%s\n' "$status" > "$root/entered/exit-status"
 docker inspect "$tag-runner" | jq '.[0].State|{Status,ExitCode,OOMKilled,StartedAt,FinishedAt}' > "$root/entered/final-state.json"
 # PostgreSQL data and all containers remain recoverable; no reset/removal.
 printf 'Model window sealed, exit %s: %s\n' "$status" "$root"
 exit "$status"
elif [[ "$action" == audit ]]; then
 [[ -f "$root/entered/exit-status" && "$(cat "$root/entered/exit-status")" == 0 && ! -e "$root/audit" ]] || exit 2
 mkdir -m 700 "$root/audit"
 docker run --rm --network none --read-only --cap-drop ALL --cap-add CHOWN --security-opt no-new-privileges:true \
  --mount "type=bind,src=$root/audit,dst=/audit" "$image_id" chown 10001:10001 /audit
 frozen=$(read_output private/frozen.json | jq -r .sha256)
 audit_mode=audit
 audit_prior=()
 if [[ ${#prior_mount[@]} -gt 0 ]]; then audit_mode=pending-audit; audit_prior=(-prior-directory /prior); fi
 docker run --rm --user 10001:10001 --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true --cpus 2 --memory 1536m --pids-limit 128 \
  -v "$source_dir:/src:ro" -v "$root/build:/runtime:ro" -v "$root/metadata:/metadata:ro" -v "$root/review:/review:ro" -v "$root/private:/original:ro" -v "$root/audit:/audit" "${prior_mount[@]}" "$image_id" \
  /runtime/memory-eval -mode "$audit_mode" -root /src -suite testdata/acceptance/ru-memory-v2.json -code-commit "$release" -hardware /metadata/hardware.json \
  -frozen /original/frozen.json -frozen-sha256 "$frozen" -measurements /original/measurements.json -runner-directory /original \
  -runner-binary /runtime/collector.test -runtime-pins /metadata/pins.json -golden-approval /review/approval.json -golden-public-key /review/public-key.hex \
  "${audit_prior[@]}" -output-dir /audit/report > "$root/audit.log" 2> "$root/audit-diagnostics.log"
 read_output audit/report/report.json | jq '{status,model_quality_evidence,promotion_allowed,baseline_comparable,arms,reasons}'
 exit 0
fi
exit 2

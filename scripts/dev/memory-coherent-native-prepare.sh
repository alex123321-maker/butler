#!/usr/bin/env bash
# Offline/auth0 exact whole92 source + immutable artifacts + CLOSED90 freeze.
set -euo pipefail
base=${1:?isolated issue92-native root}; label=${2:?fresh prepared label}
release=${3:?exact committed source revision}; previous=${4:?latest closed90 packet}; previous_hash=${5:?closed90 sha256}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-native-[A-Za-z0-9_-]+$ && "$label" =~ ^[a-z0-9-]+$ && "$release" =~ ^[a-f0-9]{40}$ && "$previous_hash" =~ ^[a-f0-9]{64}$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
test ! -e "$src/apps/orchestrator/internal/app/memory_coherent_compile_stub_test.go"
proof=$base/reports/$label; test ! -e "$proof"; test ! -e "$base/private/$label"; umask 077
mkdir -p "$base/reports"
# No implicit pulls, including build/control containers.
docker image inspect golang:1.27.2-bookworm pgvector/pgvector:pg17 > /dev/null
python3 "$src/scripts/dev/memory-coherent-native-history.py" prepare "$base" "$label" "$previous" "$previous_hash" > "$base/reports/$label-history-prepare-safe.json"
mkdir -p "$base/private" "$base/.cache/go-build" "$base/.cache/go-tmp" "$proof/bin"
test ! -L "$base/private"; test "$(stat -c %a "$base/private")" = 700; test "$(stat -c %u "$base/private")" = "$(id -u)"
printf '%s\n' "$src" > "$proof/source-reference"; printf '%s\n' "$release" > "$proof/source-revision"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
docker image inspect --format '{"id":{{json .Id}},"tags":{{json .RepoTags}},"digests":{{json .RepoDigests}}}' golang:1.27.2-bookworm pgvector/pgvector:pg17 > "$proof/images.jsonl"
printf '{"native_whole":true,"client_cpu":2,"client_memory_bytes":3221225472,"postgres_cpu":2,"postgres_memory_bytes":1073741824,"subnet":"10.237.214.0/28","postgres":"10.237.214.2:5432/coherent_native_test","client_network":"host_for_loopback_proxy_and_owned_PG","proxy":"http://127.0.0.1:18080","no_published_ports":true,"tmpfs_bytes":8192,"cached_images_only":true,"retained_own_PG":true,"race_build":true,"primary_context_budget":131072,"experience_context_budget":131072,"main_activation":false}\n' > "$proof/environment.json"
history_hash=$(sha256sum "$proof/history/history.json" | cut -d' ' -f1)
test "$history_hash" = "$previous_hash"
docker run --rm --pull never --network none --cpus 2 --memory 3g --pids-limit 256 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$proof:/proof" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 golang:1.27.2-bookworm bash -euo pipefail -c \
 'go build -o /proof/bin/test2json cmd/test2json
  go test -c -o /proof/bin/broker-control.test ./apps/tool-broker/internal/broker
  go build -o /proof/bin/tool-broker ./apps/tool-broker
  go test -c -o /proof/bin/extension.test ./apps/tool-extension/internal/runtime
  go test -c -o /proof/bin/http.test ./apps/tool-http/internal/runtime
  go test -race -c -o /proof/bin/app.test ./apps/orchestrator/internal/app
  go vet ./apps/orchestrator/internal/app > /proof/vet.log 2>&1
  chmod 0555 /proof/bin/*' > "$proof/build.log" 2>&1
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof/bin /proof/vet.log
common=(docker run --rm --pull never --network none --user "$(id -u):$(id -g)" --cpus 2 --memory 3g --pids-limit 256 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$proof:/proof" -v "$proof/bin:/artifacts:ro" -w /src/apps/orchestrator/internal/app
 -e BUTLER_MEMORY_READ_APP_BINARY=/artifacts/tool-broker -e BUTLER_BROKER_E2E_BINARY=/artifacts/tool-broker
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker-control.test -e BUTLER_EXTENSION_E2E_BINARY=/artifacts/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/artifacts/http.test
 -e BUTLER_COHERENT_NATIVE_HISTORY=/proof/history/history.json -e BUTLER_COHERENT_NATIVE_HISTORY_SHA256="$history_hash" -e BUTLER_COHERENT_NATIVE_PROOF=/proof)
"${common[@]}" -e BUTLER_COHERENT_NATIVE_PREPARE=1 golang:1.27.2-bookworm /artifacts/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /artifacts/app.test -test.v=test2json -test.timeout=3m -test.run '^TestNativeCoherentPrepareWholeLifecycle$' > "$proof/offline-preparation.jsonl" 2> "$proof/offline-preparation.log"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestNativeCoherentPrepareWholeLifecycle") and all(.[];.Action!="fail" and .Action!="skip")' "$proof/offline-preparation.jsonl" >/dev/null
plan_hash=$(sha256sum "$proof/preseal.json" | cut -d' ' -f1)
"${common[@]}" -e BUTLER_COHERENT_NATIVE_CHECK=1 -e BUTLER_COHERENT_NATIVE_PRESEAL_SHA256="$plan_hash" golang:1.27.2-bookworm /artifacts/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /artifacts/app.test -test.v=test2json -test.timeout=3m -test.run '^TestNativeCoherentCheckWholeLifecycle$' > "$proof/offline-check.jsonl" 2> "$proof/offline-check.log"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestNativeCoherentCheckWholeLifecycle") and all(.[];.Action!="fail" and .Action!="skip")' "$proof/offline-check.jsonl" >/dev/null
python3 "$src/scripts/dev/memory-coherent-native-history.py" check-current "$proof/history/history.json" "$history_hash" > "$proof/current-after-build.json"
find "$proof/history" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/history.sha256"
sha256sum "$proof/bin/"* > "$proof/binaries.sha256"
sha256sum "$proof/source-reference" "$proof/source-revision" "$proof/source.sha256" "$proof/images.jsonl" "$proof/environment.json" "$proof/current-before-build.json" "$proof/history.sha256" "$proof/binaries.sha256" "$proof/preseal.json" "$proof/scheduled-before-auth.json" "$proof/offline-preparation.jsonl" "$proof/offline-check.jsonl" "$proof/vet.log" > "$proof/prepared-pins.sha256"
printf 'PASS\n' > "$proof/prepared"
sha256sum "$proof/preseal.json" "$proof/prepared-pins.sha256"

#!/usr/bin/env bash
# Isolated owning regressions; no production SQL, provider, owner message or replay.
set -euo pipefail
root=${1:?owned server-browser-outcome root}
mode=${2:-all}
[[ "$mode" == all || "$mode" == browser || "$mode" == broker || "$mode" == resources ]] || exit 2
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-outcome-[0-9]{8}$ ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
mkdir -p "$root/reports" "$root/.cache/go-tmp" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
exec 9>"$root/.cache/validation.lock"
flock -n 9
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/$run"
mkdir "$proof"
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
browser_image=$(docker image inspect --format '{{.Id}}' butler-lab-tool-browser)
pg="butler-browser-outcome-$run"
created=false
cleanup() {
 local code=$?
 if $created; then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true; docker rm -f "$pg" >/dev/null; fi
 printf '{"exit_code":%s,"new_native_model_calls":0,"replayed_baseline_actions":0}\n' "$code" > "$proof/exit.json"
}
trap cleanup EXIT
phase() {
 local label=$1 out=$2 before after code
 shift 2
 read -r before _ < /proc/uptime
 if "$@" > "$proof/$out" 2>&1; then code=0; else code=$?; fi
 read -r after _ < /proc/uptime
 awk -v label="$label" -v start="$before" -v end="$after" -v code="$code" 'BEGIN {printf "{\"phase\":\"%s\",\"duration_ms\":%.0f,\"exit_code\":%d}\n",label,(end-start)*1000,code}' >> "$proof/timings.jsonl"
 if (( code != 0 )); then tail -70 "$proof/$out"; return "$code"; fi
 printf '%s PASS\n' "$label"
}
printf 'snapshot=%s go=%s postgres=%s browser=%s\n' "$src" "$go_image" "$pg_image" "$browser_image" > "$proof/source.txt"
(cd "$src"; sha256sum configs/tools.json apps/orchestrator/internal/orchestrator/{browser_outcome,tool_handler,recovery}.go apps/orchestrator/internal/input/control.go apps/tool-browser/internal/runtime/{arguments,server,playwright_runner}.go apps/tool-browser/scripts/browser_runtime.mjs internal/actionauth/{binding,effect}.go internal/browsercontrol/store.go internal/resourceexec/types.go) > "$proof/production.sha256"
docker run -d --name "$pg" --network none --memory 1g --cpus 1 --pids-limit 128 \
 --tmpfs /var/lib/postgresql/data:rw,size=768m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=outcome_test "$pg_image" \
 -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB >/dev/null
created=true
for i in $(seq 1 30); do docker exec "$pg" pg_isready -U postgres -d outcome_test >/dev/null 2>&1 && break; sleep 1; done
docker exec "$pg" pg_isready -U postgres -d outcome_test >/dev/null
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_COMMUNICATION_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/outcome_test?sslmode=disable'
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/outcome_test?sslmode=disable'
 -e BUTLER_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/outcome_test?sslmode=disable')
if [[ "$mode" == all || "$mode" == resources ]]; then
phase resources resource-regression.jsonl docker run "${args[@]}" "$go_image" go test -json -race -p 1 -count=1 -timeout 3m \
 ./internal/resourceexec ./apps/tool-broker/internal/broker \
 -skip '^TestSingleTabResourceActualChromiumNativeHostIntegration$' \
 -run '^TestResourceBrokerRuntime|^TestResourceBrokerConflict|^TestResourceBrokerLost|^TestResourceClaim|^TestResourceMigration|^TestResourceForeign|^TestResourceRuntime|^TestRuntimePrivate|^TestAuthorityHTTP|^TestBrowserResource|^TestSingleTabResource'
if grep -q '"Action":"skip"' "$proof/resource-regression.jsonl"; then exit 1; fi
if [[ "$mode" == resources ]]; then
 sha256sum "$proof/resource-regression.jsonl" "$proof/timings.jsonl" > "$proof/reports.sha256"
 printf 'Resource compatibility regression PASS. proof=%s\n' "$proof"
 exit 0
fi
fi
if [[ "$mode" == all ]]; then
phase pg_race pg-race.jsonl docker run "${args[@]}" "$go_image" go test -json -race -p 1 -count=1 -timeout 5m \
 ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/input ./internal/actionauth ./apps/tool-browser/internal/runtime ./apps/tool-broker/internal/registry \
 -run '^TestUnknownBrowser|^TestFrozenObservation|^TestUncertainBrowserObservationCheckpoints|^TestDurableUnknownEffectAndKnownResultRecoveryIntegration$|^TestStaleExecutorCannotPublishDurableAuthorityIntegration$|^TestBrowserGetFormReadKeepsUnknownWriteBlockedIntegration$|^TestBrowserLocalPreparationJournalAndBudgetIntegration$|^TestBrowserContractsRequireExplicitSessionAndSafeAuth$'
if grep -q '"Action":"skip"' "$proof/pg-race.jsonl"; then grep '"Action":"skip"' "$proof/pg-race.jsonl"; exit 1; fi
for required in TestUnknownBrowserObservesOnceThenBlocksWithoutModelResume TestFrozenObservationKeepsUnknownWriteBlockedIntegration TestUncertainBrowserObservationCheckpointsNeverResumeIntegration; do
 grep -q "\"Action\":\"pass\".*\"Test\":\"$required\"" "$proof/pg-race.jsonl"
done
phase vet vet.log docker run "${args[@]}" "$go_image" go vet ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/input ./internal/actionauth ./internal/browsercontrol ./internal/resourceexec ./apps/tool-browser/... ./apps/tool-broker/...
fi
phase compile_browser compile-browser.log docker run "${args[@]}" -e CGO_ENABLED=0 "$go_image" go test -c -o "/cache/outcome-browser-$run.test" ./apps/tool-browser/internal/runtime
phase compile_broker compile-broker.log docker run "${args[@]}" -e CGO_ENABLED=0 "$go_image" go test -c -o "/cache/outcome-broker-$run.test" ./apps/tool-broker/internal/broker
browser_args=(--rm --user "$(id -u):$(id -g)" --memory 2g --cpus 2 --pids-limit 256 --shm-size 256m --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v "$src/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" -v "$src/configs:/app/configs:ro"
 -e BUTLER_BROWSER_E2E=1 -e BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/outcome_test?sslmode=disable')
if [[ "$mode" != broker ]]; then
phase chromium chromium.log docker run "${browser_args[@]}" -w /src/apps/tool-browser/internal/runtime --entrypoint "/cache/outcome-browser-$run.test" "$browser_image" -test.v -test.timeout=180s -test.run '^TestBrowserSessionE2E$|^TestBrowserGetFormChromium$'
fi
phase broker_chromium broker-chromium.log docker run "${browser_args[@]}" -w /src/apps/tool-broker/internal/broker -e BUTLER_BROWSER_E2E_BINARY="/cache/outcome-browser-$run.test" --entrypoint "/cache/outcome-broker-$run.test" "$browser_image" -test.v -test.timeout=180s -test.run '^TestResourceBrokerFrozenObservationAfterOpaqueClickChromiumIntegration$|^TestResourceBrokerActualChromiumPageCASIntegration$'
if grep -q -- '--- SKIP:' "$proof/"*.log; then exit 1; fi
sha256sum "$proof/broker-chromium.log" "$proof/timings.jsonl" > "$proof/reports.sha256"
if [[ "$mode" != broker ]]; then sha256sum "$proof/chromium.log" >> "$proof/reports.sha256"; fi
if [[ "$mode" == all ]]; then sha256sum "$proof/pg-race.jsonl" "$proof/resource-regression.jsonl" >> "$proof/reports.sha256"; fi
printf 'Required mode=%s observer/quarantine checks PASS. proof=%s\n' "$mode" "$proof"

#!/usr/bin/env bash
set -euo pipefail
root=${1:?owned browser-owner test root}
[[ "$root" == /srv/storage/butler/agents/server-browser-owners-20261029 ]] || exit 2
src=/tmp/butler-browser-pre-input-20261029
test -s "$src/internal/actionauth/browser_preparation_rejection.go"
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/pre-input-$run";mkdir "$proof"
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
browser_image=$(docker image inspect --format '{{.Id}}' butler-lab-tool-browser)
pg="butler-pre-input-$run"
created=false
cleanup(){ local code=$?;if $created;then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true;docker rm -f "$pg" >/dev/null;fi;printf '{"exit_code":%d,"native_model_calls":0}\n' "$code" > "$proof/exit.json"; }
trap cleanup EXIT
docker run -d --name "$pg" --network none --memory 1g --cpus 1 --pids-limit 128 --tmpfs /var/lib/postgresql/data:rw,size=768m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=pre_input_test "$pg_image" -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB >/dev/null
created=true
for i in $(seq 1 30);do docker exec "$pg" pg_isready -U postgres -d pre_input_test >/dev/null 2>&1 && break;sleep 1;done
docker exec "$pg" pg_isready -U postgres -d pre_input_test >/dev/null
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/pre_input_test?sslmode=disable')
phase(){ local name=$1 before after code;shift;read -r before _ < /proc/uptime;if "$@" > "$proof/$name.log" 2>&1;then code=0;else code=$?;fi;read -r after _ < /proc/uptime;awk -v phase="$name" -v a="$before" -v b="$after" -v code="$code" 'BEGIN{printf "{\"phase\":\"%s\",\"duration_ms\":%.0f,\"exit_code\":%d}\n",phase,(b-a)*1000,code}' >> "$proof/timings.jsonl";if ((code));then tail -65 "$proof/$name.log";return "$code";fi;printf '%s PASS\n' "$name"; }
phase pg_race docker run "${args[@]}" "$go_image" go test -json -race -p 1 -count=1 -timeout 4m ./internal/actionauth ./apps/tool-browser/internal/runtime ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/registry \
 -run '^TestOwnerOutcomePreparation|^TestOwnerOutcomeStoppedLocalPreparation|^TestOwnerOutcomePinnedLocal|^TestLocalBrowserPreparation|^TestBrowserLocalPreparationJournalAndBudgetIntegration$|^TestBrowserContractsRequire'
if grep -q '"Action":"skip"' "$proof/pg_race.log";then exit 1;fi
for required in TestOwnerOutcomePreparationRejectionPostgresIntegration TestOwnerOutcomePreparationBrokerPreservesFixedNoInputDiagnostic TestOwnerOutcomePreparationPreInputGuardsAreVersioned TestOwnerOutcomeStoppedLocalPreparationProvesOnlyNoWriteIntegration TestBrowserLocalPreparationJournalAndBudgetIntegration;do grep -q "\"Action\":\"pass\".*\"Test\":\"$required\"" "$proof/pg_race.log";done
phase vet docker run "${args[@]}" "$go_image" go vet ./internal/actionauth ./apps/tool-browser/... ./apps/tool-broker/...
phase compile_browser docker run "${args[@]}" -e CGO_ENABLED=0 "$go_image" go test -c -o /cache/pre-input-browser.test ./apps/tool-browser/internal/runtime
phase compile_broker docker run "${args[@]}" -e CGO_ENABLED=0 "$go_image" go test -c -o /cache/pre-input-broker.test ./apps/tool-broker/internal/broker
runtime_args=(--rm --user "$(id -u):$(id -g)" --memory 2g --cpus 2 --pids-limit 256 --shm-size 256m --security-opt no-new-privileges:true --network "container:$pg"
 -v "$src:/src:ro" -v "$root/.cache:/cache" -v "$src/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" -v "$src/configs:/app/configs:ro"
 -e BUTLER_BROWSER_E2E=1 -e BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs -e BUTLER_BROWSER_E2E_BINARY=/cache/pre-input-browser.test)
phase chromium docker run "${runtime_args[@]}" -w /src/apps/tool-browser/internal/runtime --entrypoint /cache/pre-input-browser.test "$browser_image" -test.v -test.timeout=180s -test.run '^TestBrowserSessionE2E$'
phase broker_chromium docker run "${runtime_args[@]}" -w /src/apps/tool-broker/internal/broker --entrypoint /cache/pre-input-broker.test "$browser_image" -test.v -test.timeout=180s -test.run '^TestBrokerBrowserSequenceE2E$'
if grep -q -- '--- SKIP:' "$proof/chromium.log" "$proof/broker_chromium.log";then exit 1;fi
printf 'Pre-input rejection checks PASS; proof=%s\n' "$proof"

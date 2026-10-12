#!/usr/bin/env bash
# Disposable acceptance only. Never targets the shared Butler Compose project.
set -euo pipefail
root=${1:?isolated agent root required}
profile=${2:-core}
commit=${3:?exact git commit required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ && "$profile" =~ ^(core|full|full-faults)$ && "$commit" =~ ^[a-f0-9]{40}$ ]] || exit 2
local_oom=${4:-}
if [[ "$profile" == full-faults ]]; then
  if [[ -n "$local_oom" ]]; then
    [[ "$local_oom" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_/-]+/local-load-oom.json$ && "$local_oom" != *..* && -f "$local_oom" && ! -L "$local_oom" ]] || { printf 'Invalid separate sealed local model-load proof\n' >&2; exit 2; }
  fi
fi
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/go.mod" ]] || exit 2
run_id="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/fault-$run_id"
prefix="butler-acceptance-$run_id"
network="$prefix-net"
pg="$prefix-pg"
redis="$prefix-redis"
umask 077
mkdir -p "$proof/events" "$proof/bin" "$proof/private/postgres" /srv/storage/butler/.cache/go-tmp
printf '%s\n' "$commit" > "$proof/source-commit.txt"
(cd "$source_dir"; find . -type f -not -path './.cache/*' -print0 | sort -z | xargs -0 sha256sum) > "$proof/source-manifest.sha256"
sha256sum "$proof/source-manifest.sha256" > "$proof/source-manifest-hash.txt"
if [[ "$profile" == full-faults && -n "$local_oom" ]]; then cp "$local_oom" "$proof/local-load-oom.json"; fi
chmod 700 "$proof" "$proof/events" "$proof/private"
printf 'Private acceptance artifacts: %s\n' "$proof"
created_network=false
created_pg=false
created_redis=false
cleanup() {
  # Only resources successfully created by this invocation are removable.
  $created_pg && docker logs --tail 120 "$pg" > "$proof/private/postgres.log" 2>&1 || true
  $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
  $created_redis && docker rm -f "$redis" >/dev/null 2>&1 || true
  $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
export DOCKER_CONFIG="$root/.cache/docker"
docker network create --internal "$network" >/dev/null
created_network=true
docker run -d --name "$pg" --network "$network" --network-alias acceptance-postgres \
  --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=acceptance-fixture \
  --env POSTGRES_DB=postgres --volume "$proof/private/postgres:/var/lib/postgresql/data" \
  pgvector/pgvector:pg17 -c shared_buffers=32MB -c max_connections=32 -c work_mem=4MB -c max_wal_size=64MB >/dev/null
created_pg=true
docker run -d --name "$redis" --network "$network" --network-alias acceptance-redis \
  --memory 96m --pids-limit 32 --cpus .5 --security-opt no-new-privileges:true \
  redis:7-alpine redis-server --save '' --appendonly no --maxmemory 64mb --maxmemory-policy noeviction >/dev/null
created_redis=true
for attempt in $(seq 1 60); do
  if docker exec "$pg" pg_isready -U fixture -d postgres >/dev/null 2>&1; then break; fi
  [[ "$attempt" != 60 ]] || { printf 'Disposable PostgreSQL not ready\n' >&2; exit 1; }
  sleep 1
done
for group in template ownership approval owner_stop authority effects delivery budget context memory source_index auxiliary model_pressure credential_e2e extensions extension_e2e security_content security_egress security_workspace security_redaction provider_diagnostics model_provenance generated_isolation delegated_sources resources workspace workspace_runtime delegation_reports single_tab; do
  docker exec "$pg" createdb -U fixture "butler_acceptance_${group}_test"
done
docker exec "$pg" createdb -U fixture butler_memory_backup_test
docker exec "$pg" createdb -U fixture butler_memory_restore_test

# Images, kernel and available host resources are recorded without credentials.
go_image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
pg_image=$(docker image inspect pgvector/pgvector:pg17 --format '{{.Id}}')
redis_image=$(docker image inspect redis:7-alpine --format '{{.Id}}')
browser_image='not-selected'
if [[ "$profile" == full || "$profile" == full-faults ]]; then browser_image=$(docker image inspect butler-lab-tool-browser:latest --format '{{.Id}}'); fi
kernel=$(uname -r)
cpu_count=$(getconf _NPROCESSORS_ONLN)
ram_kib=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)
lscpu --json > "$proof/hardware-cpu.json"
cpu_hash=$(sha256sum "$proof/hardware-cpu.json" | cut -d' ' -f1)
printf '{"kernel":"%s","logical_cpus":%s,"host_ram_kib":%s,"cpu_manifest_sha256":"%s","go_image":"%s","postgres_image":"%s","redis_image":"%s","browser_image":"%s","go":"1.27.2","core_test_memory_limit_bytes":1610612736,"database_memory_limit_bytes":536870912,"effects":"synthetic_fixture_only"}\n' \
  "$kernel" "$cpu_count" "$ram_kib" "$cpu_hash" "$go_image" "$pg_image" "$redis_image" "$browser_image" > "$proof/hardware.json"
base_dsn='postgres://fixture:acceptance-fixture@acceptance-postgres:5432/butler_acceptance_template_test?sslmode=disable'
test_env=(--env "BUTLER_TEST_POSTGRES_URL=$base_dsn" --env "BUTLER_TEST_GOALS_POSTGRES_URL=$base_dsn"
  --env "BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=$base_dsn" --env "BUTLER_TEST_COMPLETION_POSTGRES_URL=$base_dsn"
  --env "BUTLER_TEST_MEMORY_POSTGRES_URL=$base_dsn" --env "BUTLER_TEST_SEMANTIC_POSTGRES_URL=$base_dsn"
  --env BUTLER_TEST_REDIS_URL=redis://acceptance-redis:6379/1)
go_container=(docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 128 --cpus 2
  --security-opt no-new-privileges:true --volume "$source_dir:/src:ro" --volume "$proof:/proof"
  --volume /srv/storage/butler/.cache:/cache --workdir /src
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2
  --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp)
"${go_container[@]}" golang:1.27.2-bookworm go test -race ./internal/acceptance ./cmd/acceptance -count=1 > "$proof/harness.log" 2>&1
"${go_container[@]}" golang:1.27.2-bookworm go build -o /proof/bin/acceptance ./cmd/acceptance
"${go_container[@]}" golang:1.27.2-bookworm /proof/bin/acceptance -mode corpus -root /src -output /proof/russian-corpus.json
"${go_container[@]}" golang:1.27.2-bookworm /proof/bin/acceptance -mode inputs -split holdout -root /src -output /proof/holdout-inputs.json

# The report records missing model observations/independent review as NO_GO.
# It does not copy fixture golden values into alleged model observations.
printf '{"status":"NO_GO","actual_model_calls":0,"cases":240,"source_groups":30,"development":96,"tuning":64,"holdout":80,"eligible_model_cases":236,"eligible_holdout_cases":76,"secret_cases_blocked_before_model":4,"reason":"Independent signed golden approval and actual pinned-model observations absent. Existing local quality reports do not evaluate this corpus. No model actions or single/multi-agent comparison executed."}\n' > "$proof/model-no-go.json"
core_mode=run
if [[ "$profile" == full-faults ]]; then core_mode=fault-run; fi
set +e
"${go_container[@]}" "${test_env[@]}" golang:1.27.2-bookworm /proof/bin/acceptance \
  -mode "$core_mode" -root /src -events /proof/events -profile core -commit "$commit" \
  -hardware /proof/hardware.json -output /proof/core-report.json > "$proof/core.log" 2>&1
core_exit=$?
set -e

if [[ "$profile" == full || "$profile" == full-faults ]]; then
  # Compile fixed test binaries and test2json for the actual Chromium image.
  # No npm install or writable source checkout is needed inside that image.
  "${go_container[@]}" golang:1.27.2-bookworm go build -o /proof/bin/test2json cmd/test2json
  "${go_container[@]}" golang:1.27.2-bookworm go test -c -race -o /proof/bin/browser.test ./apps/tool-browser/internal/runtime
  "${go_container[@]}" golang:1.27.2-bookworm go test -c -race -o /proof/bin/broker.test ./apps/tool-broker/internal/broker
  "${go_container[@]}" golang:1.27.2-bookworm go test -c -race -o /proof/bin/app.test ./apps/orchestrator/internal/app
  "${go_container[@]}" golang:1.27.2-bookworm go test -c -race -o /proof/bin/extension.test ./apps/tool-extension/internal/runtime
  "${go_container[@]}" golang:1.27.2-bookworm go build -o /proof/bin/tool-broker ./apps/tool-broker
  if [[ "$profile" == full-faults ]]; then
    "${go_container[@]}" --env CGO_ENABLED=0 golang:1.27.2-bookworm go build -o /proof/bin/guarded-native-host ./apps/browser-bridge/cmd/guarded
  fi
  browser_image=$(docker image inspect butler-lab-tool-browser:latest --format '{{.Id}}')
  printf '%s\n' "$browser_image" > "$proof/browser-image-id.txt"
  # Embedded helper validation must see this exact snapshot, while Playwright
  # dependencies come from the recorded browser image. Never replace live image
  # files or add node_modules to the source snapshot.
  cp -a "$source_dir" "$proof/browser-workspace"
  ln -s /app/apps/tool-browser/node_modules "$proof/browser-workspace/apps/tool-browser/node_modules"
  browser_container=(docker run --rm --user "$(id -u):$(id -g)" --network "$network" --memory 1536m --pids-limit 192 --cpus 2
    --read-only --tmpfs /tmp:rw,size=384m --security-opt no-new-privileges:true --cap-drop ALL
    --volume "$proof/browser-workspace:/fixture:ro" --volume "$proof/bin:/proof/bin:ro" --workdir /fixture
    --env BUTLER_BROWSER_E2E=1 --env BUTLER_BROWSER_E2E_SCRIPT=/fixture/apps/tool-browser/scripts/browser_runtime.mjs
    --env BUTLER_BROWSER_E2E_BINARY=/proof/bin/browser.test --env BUTLER_BROKER_E2E_BINARY=/proof/bin/tool-broker
    --env BUTLER_ACTIONAUTH_E2E=1
    --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:acceptance-fixture@acceptance-postgres:5432/butler_acceptance_credential_e2e_test?sslmode=disable'
    --entrypoint /proof/bin/test2json)
  set +e
  "${browser_container[@]}" "$browser_image" -t -p github.com/butler/butler/apps/tool-browser/internal/runtime \
    /proof/bin/browser.test -test.v=test2json -test.timeout=120s \
    '-test.run=^(TestBrowserSessionE2E|TestSessionGuardsBeforeEffect|TestExclusiveControlRejectsConcurrentEffect|TestProcessFailureInvalidatesAllSessions|TestCredentialArgumentsRequireMatchingBrokerResolution|TestCookieScopeCheckedBeforeSecretInjection)$' > "$proof/events/browser.jsonl"
  "${browser_container[@]}" "$browser_image" -t -p github.com/butler/butler/apps/tool-broker/internal/broker \
    /proof/bin/broker.test -test.v=test2json -test.timeout=120s \
    '-test.run=^(TestBrokerBrowserSequenceE2E|TestDirectBrokerCallsCannotBypassActionAuthorization|TestReflectedCredentialIsMaskedBeforeModelBoundary)$' > "$proof/events/broker-browser.jsonl"
  "${browser_container[@]}" "$browser_image" -t -p github.com/butler/butler/apps/orchestrator/internal/app \
    /proof/bin/app.test -test.v=test2json -test.timeout=120s '-test.run=^TestApprovedCredentialBrokerBrowserE2E$' > "$proof/events/credential-e2e.jsonl"
  "${go_container[@]}" --env BUTLER_EXTENSION_E2E=1 --env BUTLER_EXTENSION_E2E_BINARY=/proof/bin/extension.test \
    --env BUTLER_BROKER_E2E_BINARY=/proof/bin/tool-broker \
    --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:acceptance-fixture@acceptance-postgres:5432/butler_acceptance_extension_e2e_test?sslmode=disable' \
    golang:1.27.2-bookworm /proof/bin/test2json -t -p github.com/butler/butler/apps/orchestrator/internal/app \
    /proof/bin/app.test -test.v=test2json -test.timeout=150s '-test.run=^TestInstalledProjectExtensionRealBrokerApprovalE2E$' > "$proof/events/extension-e2e.jsonl"
  if [[ "$profile" == full-faults ]]; then
    "${browser_container[@]}" --env DISPLAY=:91 --env BUTLER_SINGLE_TAB_E2E=1 \
      --env BUTLER_SINGLE_TAB_HOST_BINARY=/proof/bin/guarded-native-host \
      --env BUTLER_SINGLE_TAB_FIXTURE_SCRIPT=/fixture/apps/browser-bridge/scripts/guarded_chromium_fixture.mjs \
      --env BUTLER_SINGLE_TAB_EXTENSION=/fixture/web/extensions/chromium-butler \
      --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:acceptance-fixture@acceptance-postgres:5432/butler_acceptance_single_tab_test?sslmode=disable' \
      --entrypoint sh "$browser_image" -c 'Xvfb :91 -screen 0 1280x900x24 -nolisten tcp >/tmp/xvfb.log 2>&1 & exec /proof/bin/test2json -t -p github.com/butler/butler/apps/tool-broker/internal/broker /proof/bin/broker.test -test.v=test2json -test.timeout=150s -test.run="^TestSingleTabResourceActualChromiumNativeHostIntegration$"' > "$proof/events/single-tab.jsonl"
  fi
  set -e

  # Actual backup before deletion, then restore the old PG contents while keeping
  # the newest independent anchor. Both databases are fresh in this disposable PG.
  for phase in seed delete restore; do
    if [[ "$phase" == restore ]]; then
      docker exec -i "$pg" pg_restore -U fixture -d butler_memory_restore_test --exit-on-error < "$proof/private/pre-delete.dump"
      backup_db=butler_memory_restore_test
    else
      backup_db=butler_memory_backup_test
    fi
    set +e
    "${go_container[@]}" --env "BUTLER_TEST_MEMORY_BACKUP_POSTGRES_URL=postgres://fixture:acceptance-fixture@acceptance-postgres:5432/$backup_db?sslmode=disable" \
      --env "BUTLER_TEST_MEMORY_BACKUP_PHASE=$phase" --env BUTLER_TEST_MEMORY_BACKUP_MANIFEST=/proof/private/deletions.log \
      golang:1.27.2-bookworm go test -json -race -count=1 -run '^TestActualPostgresDumpRestoreDeletionIntegration$' ./internal/memory/catalog > "$proof/events/backup-$phase.jsonl"
    phase_exit=$?
    set -e
    [[ "$phase_exit" == 0 ]] || { printf 'Backup acceptance phase failed: %s\n' "$phase" >&2; break; }
    if [[ "$phase" == seed ]]; then
      docker exec "$pg" pg_dump -U fixture -d butler_memory_backup_test -Fc > "$proof/private/pre-delete.dump"
      sha256sum "$proof/private/pre-delete.dump" > "$proof/backup-sha256.txt"
    fi
  done
fi
docker stats --no-stream --format '{{json .}}' "$pg" "$redis" > "$proof/container-resources.jsonl"
report_mode=report
report_profile=$profile
extra_report_args=()
if [[ "$profile" == full-faults ]]; then
  report_mode=fault-report
  report_profile=full
  extra_report_args=(-local-load-oom /proof/local-load-oom.json)
fi
"${go_container[@]}" golang:1.27.2-bookworm /proof/bin/acceptance \
  -mode "$report_mode" -root /src -events /proof/events -profile "$report_profile" -commit "$commit" \
  -hardware /proof/hardware.json -output /proof/report.json "${extra_report_args[@]}"
[[ "$core_exit" == 0 ]]

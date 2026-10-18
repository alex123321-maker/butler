#!/usr/bin/env bash
# One real controlled timeline, whole dedicated PG dump/restore and bounded ENOSPC.
# No authentication import, external network or native inference is available.
set -euo pipefail
base=${1:?isolated issue92 root}
label=${2:?fresh lifecycle report label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
printf '%s\n' "$src" > "$proof/source-reference"
printf '{"execution_host":"ssh_laba","controlled_provider":true,"native_calls":0,"provider_auth_imported":false,"network":"dedicated_internal","client_cpu":1,"client_memory_bytes":2147483648,"postgres_cpu":1,"postgres_memory_bytes":1073741824,"anchor_tmpfs_bytes":8192,"compiler":"cached_golang_1.27.2","GOTOOLCHAIN":"local","GOPROXY":"off","GOSUMDB":"off"}\n' > "$proof/environment.json"
network=butler-issue92-$label-lifecycle-net
pg=butler-issue92-$label-lifecycle-pg
client=butler-issue92-$label-lifecycle-client
created_network=false
created_pg=false
created_client=false
client_pid=
cleanup() {
 if $created_client; then
  docker logs "$client" > "$proof/client.log" 2>&1 || true
  docker rm -f "$client" >/dev/null 2>&1 || true
  created_client=false
 fi
 if [[ -n "$client_pid" ]]; then
  wait "$client_pid" 2>/dev/null || true
  client_pid=
 fi
 if $created_pg; then
  docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
  docker rm -f "$pg" >/dev/null 2>&1 || true
  created_pg=false
 fi
 if $created_network; then
  docker network rm "$network" >/dev/null 2>&1 || true
  created_network=false
 fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
docker network create --internal --subnet 10.237.199.0/24 "$network" > "$proof/network-id"
created_network=true
docker run -d --pull never --name "$pg" --network "$network" --network-alias postgres \
 --cpus 1 --memory 1g --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=coherent_lifecycle_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id"
created_pg=true
for attempt in $(seq 1 60); do
 # The entrypoint's temporary initialization server accepts Unix sockets only.
 # TCP readiness proves the final server survived that normal fast shutdown.
 docker exec "$pg" pg_isready -h 127.0.0.1 -U butler -d coherent_lifecycle_test >/dev/null 2>&1 && break
 [[ "$attempt" != 60 ]] || exit 1
 sleep 1
done
docker exec "$pg" createdb -U butler coherent_lifecycle_restore_test
docker create --pull never --name "$client" --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges --tmpfs /bounded-anchor-fault:rw,size=8192,mode=0700 \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/coherent_lifecycle_test?sslmode=disable' \
 -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker -e BUTLER_BROKER_E2E_BINARY=/proof/tool-broker \
 -e BUTLER_EXTENSION_E2E_BINARY=/proof/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/proof/http.test \
 -e BUTLER_EXPERIENCE_RECIPIENT_BROKER=1 -e BUTLER_MEMORY_COHERENT_PILOT=1 \
 -e BUTLER_MEMORY_COHERENT_PROOF=/proof/timeline \
 -e BUTLER_MEMORY_COHERENT_RESTORE_DATABASE=coherent_lifecycle_restore_test \
 -e BUTLER_MEMORY_TINY_ANCHOR=/bounded-anchor-fault \
 golang:1.27.2-bookworm bash -eu -c \
 'go build -o /proof/tool-broker ./apps/tool-broker
  go test -c -o /proof/extension.test ./apps/tool-extension/internal/runtime
  go test -c -o /proof/http.test ./apps/tool-http/internal/runtime
  go test -json -race -p 1 -count=1 -timeout 12m -run "^(TestMemoryCoherentEpisodeConsolidationRestoreIntegration|TestPrepareRetainsEnabledBrokerContractWithoutNormalizingIdentity)$" ./apps/orchestrator/internal/app ./internal/memory/experience' > "$proof/client-id"
created_client=true
docker inspect --format '{"nano_cpus":{{.HostConfig.NanoCpus}},"memory_bytes":{{.HostConfig.Memory}},"tmpfs":{{json .HostConfig.Tmpfs}}}' "$client" > "$proof/client-resources.json"
jq -e '.nano_cpus==1000000000 and .memory_bytes==2147483648 and .tmpfs["/bounded-anchor-fault"]=="rw,size=8192,mode=0700"' "$proof/client-resources.json" >/dev/null
docker inspect --format '{"nano_cpus":{{.HostConfig.NanoCpus}},"memory_bytes":{{.HostConfig.Memory}}}' "$pg" > "$proof/postgres-resources.json"
jq -e '.nano_cpus==1000000000 and .memory_bytes==1073741824' "$proof/postgres-resources.json" >/dev/null
docker start -a "$client" > "$proof/events.jsonl" 2>&1 &
client_pid=$!
driver_deadline=$((SECONDS+1200))
# docker start runs in the background; exec is invalid while the container is
# still merely created. Wait for that transition before reading any marker.
startup_deadline=$((SECONDS+60))
while :; do
 state=$(docker inspect --format '{{.State.Status}}' "$client")
 printf '%s\n' "$state" > "$proof/client-startup-state"
 case "$state" in
  running) break ;;
  created) ((SECONDS<startup_deadline)) || exit 1 ;;
  *) exit 1 ;;
 esac
 sleep 1
done

# A client root-owned 0600 marker is read through its own container. Only fixed
# paths are passed as positional arguments; neither JSON nor a schema is shell code.
await_marker() {
 local phase=$1 status=0
 local marker=/proof/timeline/$phase-ready.json
 while ((SECONDS<driver_deadline)); do
  status=0
  docker exec "$client" bash -eu -c '
   if [[ -L "$1" || ( -e "$1" && ! -f "$1" ) ]]; then exit 2; fi
   [[ -f "$1" ]] || exit 3
   [[ $(stat -c %s "$1") -le 4096 ]] || exit 2
   cat "$1"' _ "$marker" > "$proof/$phase-ready-driver.json" 2>/dev/null || status=$?
  case "$status" in
   0) return 0 ;;
   3) [[ $(docker inspect --format '{{.State.Running}}' "$client") == true ]] || return 1 ;;
   *) return 1 ;;
  esac
  sleep 1
 done
 return 1
}

complete_marker() {
 local phase=$1
 docker exec "$client" bash -eu -c '
  umask 077
  set -o noclobber
  : > "$1"
  sync -f "$1"' _ "/proof/timeline/$phase-complete"
}

await_marker backup
jq -e '.database=="coherent_lifecycle_test" and (.schema|type=="string" and test("^memory_[0-9a-f]{32}$"))' "$proof/backup-ready-driver.json" >/dev/null
schema=$(jq -r .schema "$proof/backup-ready-driver.json")
docker exec "$pg" pg_dump -U butler --no-password -Fc coherent_lifecycle_test > "$proof/before-deletion.dump"
test -s "$proof/before-deletion.dump"
docker exec -i "$pg" pg_restore --list < "$proof/before-deletion.dump" > "$proof/before-deletion.contents"
sync -f "$proof/before-deletion.dump"
sha256sum "$proof/before-deletion.dump" > "$proof/backup.sha256"
sync -f "$proof/backup.sha256"
complete_marker backup
await_marker restore
jq -e --arg schema "$schema" '.database=="coherent_lifecycle_test" and .schema==$schema' "$proof/restore-ready-driver.json" >/dev/null
test "$(docker exec "$pg" psql -X -qAt -v ON_ERROR_STOP=1 -U butler -d coherent_lifecycle_restore_test -c "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND c.relkind IN('r','p','v','m','S','f')")" = 0
sha256sum --check --status "$proof/backup.sha256"
docker exec -i "$pg" pg_restore -U butler --no-password --exit-on-error -d coherent_lifecycle_restore_test < "$proof/before-deletion.dump" > "$proof/restore.log" 2>&1
complete_marker restore
status=0
wait "$client_pid" || status=$?
client_pid=
cleanup
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestMemoryCoherentEpisodeConsolidationRestoreIntegration") and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/events.jsonl" >/dev/null
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestPrepareRetainsEnabledBrokerContractWithoutNormalizingIdentity")' "$proof/events.jsonl" >/dev/null
jq -e '.controlled_only and .pass and (.steps|length)==15 and .steps[-1].Name=="owning_source_pool_unavailable_denies_before_release"' "$proof/timeline/timeline.json" >/dev/null
jq -e '.controlled_only and .pass and .native_calls==0 and (.steps|length)==17 and .lesson_source!=""' "$proof/timeline/timeline-experience.json" >/dev/null
jq -e '.pass and .native_calls==0 and .restored and .erased_private_copies==0 and .effect_journal_unchanged and .budget_ledger_unchanged and .enospc_observed and (.steps|length)>=23 and ([.steps[].Name] as $names | ["actual_current_original_uses_earlier_preference_and_independent_verification","earlier_verified_original_to_owner_accepted_episode","earlier_preference_and_verified_episode_review_diff_activate","ordinary_broker_reads_earlier_consolidated_episode_with_current_applicability","actual_custom_dump_of_same_prewithdrawal_timeline","late_original_deletion_fences_episode_and_consolidation","fresh_custom_restore_latest_anchor_erases_original_episode_view_retains_all_charges","same_restored_timeline_actual_enospc_keeps_failed_deletion_uncommitted"] | all(. as $name | $names | index($name)!=null))' "$proof/timeline/timeline-lifecycle.json" >/dev/null
sha256sum --check --status "$proof/source.sha256"
sha256sum "$proof/source.sha256" "$proof/source-reference" "$proof/tool-broker" "$proof/extension.test" "$proof/http.test" "$proof/events.jsonl" "$proof/environment.json" "$proof/client-resources.json" "$proof/client-startup-state" "$proof/postgres-resources.json" "$proof/backup-ready-driver.json" "$proof/restore-ready-driver.json" "$proof/before-deletion.dump" "$proof/restore.log" "$proof/timeline/timeline.json" "$proof/timeline/timeline-experience.json" "$proof/timeline/timeline-lifecycle.json" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf 'Controlled shared lifecycle dump/restore and bounded ENOSPC passed; native calls 0, auth 0.\n'

#!/usr/bin/env bash
# One reviewed whole92 window. No selective retry, no main activation. The
# shared lock spans clone, collector exit, ORIGINAL durable auth CAS and probe.
set -euo pipefail
base=${1:?isolated issue92-native root}; label=${2:?prepared label}
plan_hash=${3:?reviewed preseal sha256}; pins_hash=${4:?reviewed prepared-pins sha256}
auth_binary=${5:?existing reviewed auth diagnostic binary}; auth_hash=${6:?reviewed diagnostic sha256}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-native-[A-Za-z0-9_-]+$ && "$label" =~ ^[a-z0-9-]+$ && "$plan_hash" =~ ^[a-f0-9]{64}$ && "$pins_hash" =~ ^[a-f0-9]{64}$ && "$auth_hash" =~ ^[a-f0-9]{64}$ && "$auth_binary" == /srv/storage/butler/agents/*/reports/provider-auth-current ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label; private=$base/private/$label; umask 077
test "$(cat "$proof/source-reference")" = "$src"; test "$(cat "$proof/prepared")" = PASS
test "$(sha256sum "$proof/preseal.json" | cut -d' ' -f1)" = "$plan_hash"
test "$(sha256sum "$proof/prepared-pins.sha256" | cut -d' ' -f1)" = "$pins_hash"
test "$(sha256sum "$auth_binary" | cut -d' ' -f1)" = "$auth_hash"
for pins in prepared-pins source history binaries; do sha256sum --check --status "$proof/$pins.sha256"; done
test ! -e "$proof/auth-stage-begun.json"; test ! -e "$private"; test ! -e "$proof/postgres"; test ! -e "$proof/timeline"
docker image inspect golang:1.27.2-bookworm pgvector/pgvector:pg17 > /dev/null
docker image inspect --format '{"id":{{json .Id}},"tags":{{json .RepoTags}},"digests":{{json .RepoDigests}}}' golang:1.27.2-bookworm pgvector/pgvector:pg17 > "$proof/images-before-auth.jsonl"
cmp --silent "$proof/images.jsonl" "$proof/images-before-auth.jsonl"
ip -j route show > "$proof/routes-before-auth.json"; docker network inspect $(docker network ls -q) > "$proof/networks-before-auth.json"
python3 - "$proof/routes-before-auth.json" "$proof/networks-before-auth.json" <<'PY'
import ipaddress,json,sys
owned=ipaddress.ip_network('10.237.214.0/28')
routes=json.load(open(sys.argv[1]));networks=json.load(open(sys.argv[2]))
subnets=[r['dst'] for r in routes if r.get('dst') not in (None,'default')]
subnets += [s['Subnet'] for n in networks for s in (n.get('IPAM',{}).get('Config') or []) if s.get('Subnet')]
if any(ipaddress.ip_network(s).version==4 and owned.overlaps(ipaddress.ip_network(s)) for s in subnets):
 raise SystemExit('owned214 overlaps; no resources/auth created')
PY
history_hash=$(sha256sum "$proof/history/history.json" | cut -d' ' -f1)
exec 9>/srv/storage/butler/agents/native-provider-auth.lock; flock -n 9
python3 "$src/scripts/dev/memory-coherent-native-history.py" check-current "$proof/history/history.json" "$history_hash" > "$proof/pre-auth-current-safe.json"
docker run --rm --pull never --network none --user "$(id -u):$(id -g)" --cpus 2 --memory 3g --pids-limit 256 --security-opt no-new-privileges \
 -v "$src:/src:ro" -v "$proof:/proof:ro" -v "$proof/bin:/artifacts:ro" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_MEMORY_READ_APP_BINARY=/artifacts/tool-broker -e BUTLER_BROKER_E2E_BINARY=/artifacts/tool-broker -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker-control.test \
 -e BUTLER_EXTENSION_E2E_BINARY=/artifacts/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/artifacts/http.test \
 -e BUTLER_COHERENT_NATIVE_HISTORY=/proof/history/history.json -e BUTLER_COHERENT_NATIVE_HISTORY_SHA256="$history_hash" \
 -e BUTLER_COHERENT_NATIVE_PROOF=/proof -e BUTLER_COHERENT_NATIVE_CHECK=1 -e BUTLER_COHERENT_NATIVE_PRESEAL_SHA256="$plan_hash" \
 golang:1.27.2-bookworm /artifacts/app.test -test.v -test.timeout=3m -test.run '^TestNativeCoherentCheckWholeLifecycle$' > "$proof/pre-auth-qualified-check.txt" 2>&1
network=butler-issue92-$label-native-net; pg=butler-issue92-$label-native-pg; client=butler-issue92-$label-native
created_net=false; created_pg=false; created_client=false; cloned=false; client_pid=
handoff(){
 status=$?; trap - EXIT; set +e
 handback=0
 # Join/terminate the entire collector before reading its latest durable auth.
 if $created_client; then
  if [[ $(docker inspect --format '{{.State.Running}}' "$client") == true ]]; then docker stop -t 10 "$client" >/dev/null || handback=1; fi
  docker inspect --format '{{json .State}}' "$client" > "$proof/collector-exit-safe.json" || handback=1
  jq -e '.Running==false and (.Status=="exited" or .Status=="dead")' "$proof/collector-exit-safe.json" >/dev/null || handback=1
 fi
 if [[ -n "$client_pid" ]]; then wait "$client_pid" >/dev/null 2>&1; client_pid=; fi
 if $cloned && [[ "$handback" == 0 ]]; then
  python3 "$src/scripts/dev/memory-coherent-auth-recovery.py" "$base" "$label" "$plan_hash" > "$proof/recovery-safe.json" || handback=1
  if [[ "$handback" == 0 ]]; then python3 "$src/scripts/dev/memory-consolidation-auth.py" return "$base" "$label" > "$proof/return-safe.json" || handback=1; fi
  if [[ "$handback" == 0 ]]; then
   docker run --rm --pull never --network container:butler-lab-orchestrator-1 --env-file "$private/main-diagnose.env" -v "$auth_binary:/provider-auth:ro" golang:1.27.2-bookworm /provider-auth -action diagnose-cloudcode > "$private/main-diagnosis.json" || handback=1
   jq -e -f "$src/scripts/dev/memory-consolidation-main-auth.jq" "$private/main-diagnosis.json" >/dev/null || handback=1
   if [[ "$handback" == 0 ]]; then
    jq '{resolved,unsafe_refresh,expired:.after.expired,trace}' "$private/main-diagnosis.json" > "$proof/main-auth-safe.json"
    python3 "$src/scripts/dev/memory-consolidation-auth.py" snapshot "$base" "$label" > "$proof/main-snapshot-safe.json" || handback=1
   fi
  fi
 fi
 printf '%s\n' "$status" > "$proof/native-status"; printf '%s\n' "$handback" > "$proof/handback-status"
 if $created_pg; then docker logs "$pg" > "$proof/postgres.log" 2>&1; fi
 python3 "$src/scripts/dev/memory-coherent-native-close.py" "$base" "$label" "$plan_hash" || handback=1
 if $created_client; then docker rm -f "$client" >/dev/null; fi
 if $created_pg; then docker rm -f "$pg" >/dev/null; fi
 if $created_net; then docker network rm "$network" >/dev/null; fi
 # PG bytes/private files survive success and failure. Never chown PGDATA.
 [[ "$handback" == 0 ]] || exit 1
 exit "$status"
}
trap handoff EXIT; trap 'exit 130' INT; trap 'exit 143' TERM
mkdir "$proof/postgres"
docker network create --internal --subnet 10.237.214.0/28 "$network" > "$proof/network-id"; created_net=true
docker run -d --pull never --name "$pg" --network "$network" --ip 10.237.214.2 --cpus 2 --memory 1g --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=coherent_native_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id"; created_pg=true
for i in $(seq 1 60); do docker exec "$pg" pg_isready -h127.0.0.1 -U butler -d coherent_native_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
docker exec "$pg" createdb -U butler coherent_lifecycle_restore_test
python3 "$src/scripts/dev/memory-consolidation-auth.py" clone "$base" "$label" > "$proof/clone-safe.json"; cloned=true
docker create --pull never --name "$client" --network host --user "$(id -u):$(id -g)" --cpus 2 --memory 3g --pids-limit 256 --security-opt no-new-privileges \
 --tmpfs "/bounded-anchor-fault:rw,size=8192,mode=0700,uid=$(id -u),gid=$(id -g)" --env-file "$private/native.env" \
 -v "$src:/src:ro" -v "$proof:/proof" -v "$proof/bin:/artifacts:ro" -v "$private:/native-private" -w /src/apps/orchestrator/internal/app \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@10.237.214.2:5432/coherent_native_test?sslmode=disable' \
 -e BUTLER_MEMORY_COHERENT_PILOT=1 -e BUTLER_COHERENT_NATIVE_EXECUTE=1 -e BUTLER_COHERENT_NATIVE_PRESEAL_SHA256="$plan_hash" -e BUTLER_COHERENT_NATIVE_APPROVED_SHA256="$plan_hash" \
 -e BUTLER_COHERENT_NATIVE_HISTORY=/proof/history/history.json -e BUTLER_COHERENT_NATIVE_HISTORY_SHA256="$history_hash" -e BUTLER_COHERENT_NATIVE_PROOF=/proof \
 -e BUTLER_COHERENT_NATIVE_AUTH_IMPORT=/native-private/auth-import.value -e BUTLER_COHERENT_NATIVE_AUTH_RETURN=/native-private/auth-return.value \
 -e BUTLER_MEMORY_READ_APP_BINARY=/artifacts/tool-broker -e BUTLER_BROKER_E2E_BINARY=/artifacts/tool-broker -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/artifacts/broker-control.test \
 -e BUTLER_EXTENSION_E2E_BINARY=/artifacts/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/artifacts/http.test \
 -e BUTLER_MEMORY_COHERENT_PROOF=/proof/timeline -e BUTLER_MEMORY_COHERENT_RESTORE_DATABASE=coherent_lifecycle_restore_test -e BUTLER_MEMORY_TINY_ANCHOR=/bounded-anchor-fault \
 -e HTTP_PROXY=http://127.0.0.1:18080 -e HTTPS_PROXY=http://127.0.0.1:18080 -e ALL_PROXY=http://127.0.0.1:18080 -e NO_PROXY=localhost,127.0.0.1,::1,10.237.214.2 \
 golang:1.27.2-bookworm /artifacts/test2json -p github.com/butler/butler/apps/orchestrator/internal/app /artifacts/app.test -test.v=test2json -test.timeout=112m -test.run '^TestNativeCoherentWholeLifecycle$' > "$proof/client-id"; created_client=true
docker inspect --format '{"nano_cpus":{{.HostConfig.NanoCpus}},"memory_bytes":{{.HostConfig.Memory}},"tmpfs":{{json .HostConfig.Tmpfs}},"ports":{{json .HostConfig.PortBindings}}}' "$client" > "$proof/client-resources.json"
docker inspect --format '{"nano_cpus":{{.HostConfig.NanoCpus}},"memory_bytes":{{.HostConfig.Memory}},"ports":{{json .HostConfig.PortBindings}}}' "$pg" > "$proof/postgres-resources.json"
docker start -a "$client" > "$proof/runner.jsonl" 2> "$proof/runner.log" & client_pid=$!
deadline=$((SECONDS+6900))
await_marker(){
 local phase=$1 status
 while ((SECONDS<deadline)); do
  status=0
  docker exec "$client" bash -eu -c '[[ ! -L "$1" ]]; [[ -f "$1" ]] || exit 3; [[ $(stat -c %s "$1") -le 4096 ]]; cat "$1"' _ "/proof/timeline/$phase-ready.json" > "$proof/$phase-ready.json" 2>/dev/null || status=$?
  case "$status" in 0)return 0;;3)[[ $(docker inspect --format '{{.State.Running}}' "$client") == true ]] || return 1;;*)return 1;;esac
  sleep 2
 done
 return 1
}
complete_marker(){ docker exec "$client" bash -eu -c 'umask 077; set -o noclobber; : > "$1"; sync -f "$1"' _ "/proof/timeline/$1-complete"; }
await_marker backup
jq -e '.database=="coherent_native_test" and .schema=="coherent_native_v1"' "$proof/backup-ready.json" >/dev/null
docker exec "$pg" pg_dump -U butler --no-password -Fc coherent_native_test > "$proof/before-deletion.dump"
test -s "$proof/before-deletion.dump"; sync -f "$proof/before-deletion.dump"; sha256sum "$proof/before-deletion.dump" > "$proof/backup.sha256"; sync -f "$proof/backup.sha256"
complete_marker backup
await_marker restore
jq -e '.database=="coherent_native_test" and .schema=="coherent_native_v1"' "$proof/restore-ready.json" >/dev/null
sha256sum --check --status "$proof/backup.sha256"
test "$(docker exec "$pg" psql -X -qAt -vON_ERROR_STOP=1 -U butler -d coherent_lifecycle_restore_test -c "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND c.relkind IN('r','p','v','m','S','f')")" = 0
docker exec -i "$pg" pg_restore -U butler --no-password --exit-on-error -d coherent_lifecycle_restore_test < "$proof/before-deletion.dump" > "$proof/restore.log" 2>&1
complete_marker restore
while [[ $(docker inspect --format '{{.State.Running}}' "$client") == true ]]; do ((SECONDS<deadline)) || exit 1; sleep 2; done
status=0; wait "$client_pid" || status=$?; client_pid=
[[ "$status" == 0 ]] || exit "$status"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestNativeCoherentWholeLifecycle") and all(.[];.Action!="fail" and .Action!="skip")' "$proof/runner.jsonl" >/dev/null
# EXIT trap performs the mandatory exited-process handback and closed receipt.

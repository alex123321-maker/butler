#!/usr/bin/env bash
# New options path only: real retained PG/Broker/runtime/dump/restore/disk,
# explicitly controlled inference, auth0/native0. Never a native quality receipt.
set -euo pipefail
base=${1:?isolated issue92-native root}; label=${2:?fresh offline label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-native-[A-Za-z0-9_-]+$ && "$label" =~ ^[a-z0-9-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
test ! -e "$src/apps/orchestrator/internal/app/memory_coherent_compile_stub_test.go"
proof=$base/reports/$label; test ! -e "$proof"; umask 077
docker image inspect golang:1.27.2-bookworm pgvector/pgvector:pg17 >/dev/null
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
python3 "$src/scripts/dev/memory-coherent-native-controls.py" > "$proof/python-controls.log" 2>&1
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
ip -j route show > "$proof/routes-before.json"
docker network inspect $(docker network ls -q) > "$proof/networks-before.json"
python3 - "$proof/routes-before.json" "$proof/networks-before.json" <<'PY'
import ipaddress,json,sys
owned=ipaddress.ip_network('10.237.214.0/28')
routes=json.load(open(sys.argv[1]));networks=json.load(open(sys.argv[2]))
subnets=[r['dst'] for r in routes if r.get('dst') not in (None,'default')]
subnets += [s['Subnet'] for n in networks for s in (n.get('IPAM',{}).get('Config') or []) if s.get('Subnet')]
if any(ipaddress.ip_network(s).version==4 and owned.overlaps(ipaddress.ip_network(s)) for s in subnets):
 raise SystemExit('owned214 network/route overlaps; no resources created')
PY
printf '{"controlled_only":true,"controlled_capture_and_cost_mechanics":true,"native_calls":0,"auth":0,"client_cpu":2,"client_memory_bytes":3221225472,"postgres_cpu":2,"postgres_memory_bytes":1073741824,"network":"owned_internal_214_28","tmpfs_bytes":8192,"no_published_ports":true,"retained_failed_schema":true,"compiler":"cached_go1.27.2","all_foreground_context_budget":131072,"temporal":"actual_shared_predicate_current_qualification"}\n' > "$proof/environment.json"
network=butler-issue92-$label-native-controls-net
pg=butler-issue92-$label-native-controls-pg
client=butler-issue92-$label-native-controls-client
created_net=false; created_pg=false; created_client=false; client_pid=
cleanup(){
 if $created_client;then docker logs "$client" > "$proof/client.log" 2>&1 || true;docker rm -f "$client" >/dev/null 2>&1 || true;created_client=false;fi
 if [[ -n "$client_pid" ]];then wait "$client_pid" 2>/dev/null || true;client_pid=;fi
 if $created_pg;then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true;docker rm -f "$pg" >/dev/null 2>&1 || true;created_pg=false;fi
 if $created_net;then docker network rm "$network" >/dev/null 2>&1 || true;created_net=false;fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
docker network create --internal --subnet 10.237.214.0/28 "$network" > "$proof/network-id";created_net=true
docker run -d --pull never --name "$pg" --network "$network" --ip 10.237.214.2 --cpus 2 --memory 1g --pids-limit 128 --security-opt no-new-privileges \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=coherent_native_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id";created_pg=true
for i in $(seq 1 60);do docker exec "$pg" pg_isready -h127.0.0.1 -U butler -d coherent_native_test >/dev/null 2>&1 && break;[[ "$i" != 60 ]] || exit 1;sleep 1;done
docker exec "$pg" createdb -U butler coherent_lifecycle_restore_test
docker create --pull never --name "$client" --network "$network" --cpus 2 --memory 3g --pids-limit 256 --security-opt no-new-privileges \
 --tmpfs /bounded-anchor-fault:rw,size=8192,mode=0700 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@10.237.214.2:5432/coherent_native_test?sslmode=disable' \
 -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker -e BUTLER_BROKER_E2E_BINARY=/proof/tool-broker \
 -e BUTLER_EXTENSION_E2E_BINARY=/proof/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/proof/http.test \
 -e BUTLER_EXPERIENCE_RECIPIENT_BROKER=1 -e BUTLER_MEMORY_COHERENT_PILOT=1 -e BUTLER_COHERENT_NATIVE_OPTIONS_CONTROL=1 \
 -e BUTLER_MEMORY_COHERENT_PROOF=/proof/timeline -e BUTLER_MEMORY_COHERENT_RESTORE_DATABASE=coherent_lifecycle_restore_test -e BUTLER_MEMORY_TINY_ANCHOR=/bounded-anchor-fault \
 golang:1.27.2-bookworm bash -eu -c \
 'go build -o /proof/tool-broker ./apps/tool-broker
  go test -c -o /proof/extension.test ./apps/tool-extension/internal/runtime
  go test -c -o /proof/http.test ./apps/tool-http/internal/runtime
  go vet ./apps/orchestrator/internal/app > /proof/vet.log 2>&1
  go test -json -race -p 1 -count=1 -timeout 15m -run "^(TestCoherentNative.*|TestCoherentControlledFaultNeverReportsSyntheticUsage|TestExperienceNativeForegroundConfigurationDefaultsAndPredicate)$" ./apps/orchestrator/internal/app' > "$proof/client-id";created_client=true
docker inspect --format '{"nano_cpus":{{.HostConfig.NanoCpus}},"memory_bytes":{{.HostConfig.Memory}},"tmpfs":{{json .HostConfig.Tmpfs}},"ports":{{json .HostConfig.PortBindings}}}' "$client" > "$proof/client-resources.json"
docker inspect --format '{"nano_cpus":{{.HostConfig.NanoCpus}},"memory_bytes":{{.HostConfig.Memory}},"ports":{{json .HostConfig.PortBindings}}}' "$pg" > "$proof/postgres-resources.json"
docker start -a "$client" > "$proof/events.jsonl" 2>&1 & client_pid=$!
deadline=$((SECONDS+1200))
while [[ $(docker inspect --format '{{.State.Status}}' "$client") == created ]];do ((SECONDS<deadline)) || exit 1;sleep 1;done
await_marker(){
 local phase=$1 status
 while ((SECONDS<deadline));do
  status=0
  docker exec "$client" bash -eu -c '[[ ! -L "$1" ]];[[ -f "$1" ]] || exit 3;[[ $(stat -c %s "$1") -le 4096 ]];cat "$1"' _ "/proof/timeline/$phase-ready.json" > "$proof/$phase-ready.json" 2>/dev/null || status=$?
  case "$status" in 0)return 0;;3)[[ $(docker inspect --format '{{.State.Running}}' "$client") == true ]] || return 1;;*)return 1;;esac
  sleep 1
 done
 return 1
}
complete_marker(){ docker exec "$client" bash -eu -c 'umask 077;set -o noclobber;: > "$1";sync -f "$1"' _ "/proof/timeline/$1-complete";}
await_marker backup
jq -e '.database=="coherent_native_test" and .schema=="coherent_native_v1"' "$proof/backup-ready.json" >/dev/null
docker exec "$pg" pg_dump -U butler --no-password -Fc coherent_native_test > "$proof/before-deletion.dump"
test -s "$proof/before-deletion.dump";sync -f "$proof/before-deletion.dump";sha256sum "$proof/before-deletion.dump" > "$proof/backup.sha256";sync -f "$proof/backup.sha256"
complete_marker backup
await_marker restore
jq -e '.database=="coherent_native_test" and .schema=="coherent_native_v1"' "$proof/restore-ready.json" >/dev/null
sha256sum --check --status "$proof/backup.sha256"
test "$(docker exec "$pg" psql -X -qAt -vON_ERROR_STOP=1 -U butler -d coherent_lifecycle_restore_test -c "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT IN('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%' AND c.relkind IN('r','p','v','m','S','f')")" = 0
docker exec -i "$pg" pg_restore -U butler --no-password --exit-on-error -d coherent_lifecycle_restore_test < "$proof/before-deletion.dump" > "$proof/restore.log" 2>&1
complete_marker restore
status=0;wait "$client_pid" || status=$?;client_pid=
docker exec "$pg" psql -X -qAt -vON_ERROR_STOP=1 -U butler -d coherent_native_test -c "SELECT jsonb_build_object('retained_schema',to_regnamespace('coherent_native_v1') IS NOT NULL,'domain_receipts',(SELECT count(*) FROM public.issue92_native_windows),'auth_rows',(SELECT count(*) FROM coherent_native_v1.system_settings WHERE is_secret OR key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'))" > "$proof/retained-domain-safe.json"
cleanup
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm bash -eu -c 'find /proof -path /proof/postgres -prune -o -exec chown -h "$1:$2" {} +' _ "$(id -u)" "$(id -g)"
[[ "$status" == 0 ]] || exit "$status"
jq -s -e 'any(.[];.Action=="pass" and .Test=="TestCoherentNativeOptionsOfflineLifecycleIntegration") and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/events.jsonl" >/dev/null
jq -e '.controlled_only and .pass and .native_calls==0 and (.steps|length)==25 and .restored and .erased_private_copies==0 and .effect_journal_unchanged and .budget_ledger_unchanged and .enospc_observed' "$proof/timeline/timeline-lifecycle.json" >/dev/null
jq -e '.retained_schema and .domain_receipts==1 and .auth_rows==0' "$proof/retained-domain-safe.json" >/dev/null
jq -e '.controlled_only and .actual_native_calls==0 and .auth==0 and .controlled_method_entries>0 and .scripted_usage_reported==false' "$proof/controlled-capture/controlled-provider-kind.json" >/dev/null
sha256sum --check --status "$proof/source.sha256"
sha256sum "$proof/source.sha256" "$proof/events.jsonl" "$proof/vet.log" "$proof/python-controls.log" "$proof/environment.json" "$proof/client-resources.json" "$proof/postgres-resources.json" "$proof/before-deletion.dump" "$proof/restore.log" "$proof/retained-domain-safe.json" "$proof/timeline/timeline-lifecycle.json" "$proof/controlled-capture/full-quoted-costs.json" "$proof/controlled-capture/controlled-provider-kind.json" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf 'Offline explicit options control passed; native0/auth0; independent quality remains unqualified.\n'

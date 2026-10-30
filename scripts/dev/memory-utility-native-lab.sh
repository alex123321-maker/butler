#!/bin/sh
# Native consumption of the complete prospective corpus. Never reuses a window.
set -eu
umask 077
r=${1:?fresh native root}; tested=${2:?successful controlled root}; source=${3:?tested full commit}
predecessor=${4:-}
completed_predecessor=${5:-}
case "$r" in /srv/gitea/butler-acceptance/issue115-utility-native-*) ;; *) exit 2;; esac
case "$tested" in /srv/gitea/butler-acceptance/issue115-utility-trace-whole-*|/srv/gitea/butler-acceptance/issue115-utility-trace-repair-*) ;; *) exit 2;; esac
test ${#source} = 40
grep -q '"exit_code":0' "$tested/reports/exit.json"
if [ -f "$tested/reports/repair-summary.json" ]; then
 python3 - "$tested" "$source" <<'PY'
import hashlib,json,pathlib,sys
r=pathlib.Path(sys.argv[1]);g=json.loads((r/'reports/repair-summary.json').read_text())
assert g['source']==sys.argv[2] and g['controlled_slots']==72 and g['test_failures']==0 and g['test_skips']==0 and g['native_requests']==0
assert hashlib.sha256((r/'reports/tests.jsonl').read_bytes()).hexdigest()==g['tests_sha256']
assert hashlib.sha256((r/'reports/build.log').read_bytes()).hexdigest()==g['unit_and_race_build_log_sha256']
PY
 test -n "$completed_predecessor"
fi
mkdir "$r" "$r/out" "$r/bin" "$r/previous" "$r/pgdata"
cp -a --reflink=auto "$tested/source" "$r/source"
for file in app.test broker extension.test http.test test2json; do cp -p "$tested/reports/$file" "$r/bin/$file"; done
old=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
prior=$old/skill-broker-20261029-v1
for file in abc.json force.json prompt.json context.json; do cp -p "$prior/previous/$file" "$r/previous/$file"; done
cp -p "$prior/out/result.json" "$r/previous/skill-broker.json"
if [ -f "$tested/reports/repair-summary.json" ]; then
 cp -p "$tested/reports/repair-summary.json" "$r/previous/controlled-repair.json"
fi
if [ -n "$predecessor" ]; then
 case "$predecessor" in /srv/gitea/butler-acceptance/issue115-utility-native-*/recovery-report.json) ;; *) exit 2;; esac
 cp -p "$predecessor" "$r/previous/predecessor-interrupted.json"
fi
if [ -n "$completed_predecessor" ]; then
 case "$completed_predecessor" in /srv/gitea/butler-acceptance/issue115-utility-native-*/public-report.json) ;; *) exit 2;; esac
 cp -p "$completed_predecessor" "$r/previous/predecessor-completed.json"
fi
image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
docker run --rm --network none --cpus 1 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --user 1000:1000 --log-driver none --tmpfs /tmp:rw,nosuid,nodev,size=128m -v "$r/source:/source:ro" -v "$r/bin:/binproof:ro" -v "$r/out:/out" -v "$r/previous:/previous:ro" -e BUTLER_UTILITY_NATIVE_FREEZE=/out/plan -e BUTLER_UTILITY_NATIVE_SOURCE="$source" -w /source/apps/orchestrator/internal/app "$image" /binproof/app.test -test.v -test.run '^TestMemoryUtilityFreezeNativeABC$' > "$r/freeze.log" 2>&1
if [ -f "$tested/reports/repair-summary.json" ]; then
 python3 - "$r" <<'PY'
import json,pathlib,sys
r=pathlib.Path(sys.argv[1]);suite=json.loads((r/'out/plan/suite.private.json').read_text());plan=json.loads((r/'out/plan/plan.json').read_text())
assert suite['version']=='butler-memory-utility-v3' and len(suite['cases'])==24 and len(plan['Slots'])==72
assert {'controlled-repair.json','predecessor-completed.json','predecessor-interrupted.json'} <= set(plan['HistoricalReceipts'])
PY
fi
net=$(basename "$r")-net
pg=$(basename "$r")-pg
helper=$(basename "$r")-runner
made_net=0; made_pg=0; cid=; started_source=0
sourcepg=issue115-native-pg-20261028-v1
cleanup() {
 status=$?; trap - EXIT
 if [ -n "$cid" ]; then
  docker top "$cid" -eo pid > "$r/final-processes.txt" 2>/dev/null || true
  docker inspect "$cid" > "$r/final-container.json" 2>/dev/null || true
  docker rm -f "$cid" >/dev/null
 fi
 if [ "$made_pg" = 1 ]; then docker stop -t 15 "$pg" > "$r/pg-stop.txt"; docker network disconnect "$net" "$pg"; fi
 if [ "$made_net" = 1 ]; then docker network rm "$net" > "$r/network-removed.txt"; fi
 if [ "$started_source" = 1 ]; then
  clients=$(docker exec "$sourcepg" psql -U postgres -p 55451 -d behavior76_test -Atc "SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()")
  printf '%s\n' "$clients" > "$r/source-remaining-clients.txt"
  if [ "$clients" = 0 ]; then docker stop -t 15 "$sourcepg" > "$r/source-stop.txt"; else status=1; fi
 fi
 printf '{"exit_code":%s,"native_window":"utility-abc-20261030-v1","previous_results_preserved":true}\n' "$status" > "$r/exit.json"
 exit "$status"
}
trap cleanup EXIT
docker network create --subnet 10.249.119.0/28 "$net" > "$r/network.id"; made_net=1
docker run -d --name "$pg" --network "$net" --cpus 1 --memory 768m --memory-swap 768m --pids-limit 128 -p 127.0.0.1:55456:5432 -e POSTGRES_USER=utility_fixture -e POSTGRES_PASSWORD=utility_fixture_only -e POSTGRES_DB=utility_native_test -v "$r/pgdata:/var/lib/postgresql/data" sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d > "$r/pg.id"; made_pg=1
for n in $(seq 1 40); do if docker exec "$pg" pg_isready -U utility_fixture -d utility_native_test >/dev/null 2>&1; then break; fi; sleep 1; done
test "$(docker inspect -f '{{.Id}}' "$sourcepg")" = "$(cat "$old/native-pg-container.txt")"
if [ "$(docker inspect -f '{{.State.Running}}' "$sourcepg")" = false ]; then docker start "$sourcepg" > "$r/source-start.txt"; started_source=1; fi
for n in $(seq 1 40); do if docker exec "$sourcepg" pg_isready -U postgres -p 55451 >/dev/null 2>&1; then break; fi; sleep 1; done
cid=$(docker create --name "$helper" --network host --cpus 1 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --user 1000:1000 --log-driver none --tmpfs /tmp:rw,nosuid,nodev,size=512m -v "$r/source:/source:ro" -v "$r/bin:/binproof:ro" -v "$r/out:/out" -v "$r/out/plan:/plan:ro" -v "$r/previous:/previous:ro" -v "$old/auth-source/settings.key:/private/settings.key:ro" -e HTTPS_PROXY=http://127.0.0.1:18080 -e BUTLER_UTILITY_NATIVE=utility-abc-20261030-v1 -e BUTLER_UTILITY_NATIVE_SOURCE="$source" -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://utility_fixture:utility_fixture_only@127.0.0.1:55456/utility_native_test?sslmode=disable' -e BUTLER_BROKER_E2E_BINARY=/binproof/broker -e BUTLER_EXTENSION_E2E_BINARY=/binproof/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/binproof/http.test -e GOMAXPROCS=1 -w /source/apps/orchestrator/internal/app "$image" /bin/sleep 15000)
docker start "$cid" > /dev/null
docker inspect "$cid" > "$r/container-before-auth.json"
python3 - "$r" "$source" <<'PY'
# Stdlib-only host kernel attestation, not product code or credential handling.
import sys,json,pathlib,hashlib
r=pathlib.Path(sys.argv[1]);d=json.loads((r/'container-before-auth.json').read_text())[0];h=d['HostConfig'];p=d['State']['Pid']
assert d['State']['Running'] and d['Config']['User']=='1000:1000' and not h['Privileged'] and h['ReadonlyRootfs'] and h['NetworkMode']=='host' and h['PidMode']==''
assert h['NanoCpus']==1000000000 and h['Memory']==3221225472 and h['MemorySwap']==3221225472 and h['PidsLimit']==256 and 'ALL' in h['CapDrop']
assert any(x.startswith('no-new-privileges') for x in h['SecurityOpt'])
mounts={x['Destination']:x for x in d['Mounts']};assert set(mounts)=={'/source','/binproof','/out','/plan','/previous','/private/settings.key'} and all(not x['RW'] for k,x in mounts.items() if k!='/out')
status=dict(x.split(':',1) for x in pathlib.Path(f'/proc/{p}/status').read_text().splitlines() if ':' in x)
assert int(status['CapEff'].strip(),16)==0 and status['NoNewPrivs'].strip()=='1' and status['Seccomp'].strip()=='2'
assert pathlib.Path(f'/proc/{p}/ns/pid').readlink()!=pathlib.Path('/proc/self/ns/pid').readlink()
cg=next(x.split('::',1)[1] for x in pathlib.Path(f'/proc/{p}/cgroup').read_text().splitlines() if x.startswith('0::'));group=pathlib.Path('/sys/fs/cgroup')/cg.lstrip('/');assert d['Id'] in str(group)
v={k:(group/k).read_text().strip() for k in ['cpu.max','memory.max','memory.swap.max','pids.max']};assert v=={'cpu.max':'100000 100000','memory.max':'3221225472','memory.swap.max':'0','pids.max':'256'}
proof={'source':sys.argv[2],'pid':p,'cgroup':str(group),'actual_kernel_values':v,'binary_sha256':hashlib.sha256((r/'bin/app.test').read_bytes()).hexdigest(),'plan_sha256':hashlib.sha256((r/'out/plan/plan.json').read_bytes()).hexdigest(),'native_requests_before_proof':0}
(r/'out/kernel-verified.json').write_text(json.dumps(proof)+'\n')
PY
journal_schema=$(python3 -c 'import json,sys;print("utility_abc_"+json.load(open(sys.argv[1]))["SuiteSHA256"][:16])' "$r/out/plan/plan.json")
set +e
docker exec "$cid" /binproof/test2json -t -p github.com/butler/butler/apps/orchestrator/internal/app /binproof/app.test -test.v -test.timeout=14500s -test.run '^TestMemoryUtilityWholeNativeABC$' > "$r/native.jsonl" 2>&1
native_status=$?
set -e
docker exec "$sourcepg" psql -U postgres -p 55451 -d behavior76_test -Atc "SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY seq),'[]') FROM $journal_schema.entries e" > "$r/out/journal-complete.private.json"
exit "$native_status"

#!/bin/sh
# Operator-only, synthetic full V3 comparison. One API key arrives on private stdin.
set -eu
umask 077
r=${1:?fresh native root}; tested=${2:?controlled gate}; source=${3:?full commit}
reference=/srv/gitea/butler-acceptance/issue115-utility-native-20261030-v3-r1
case "$r" in /srv/gitea/butler-acceptance/issue115-utility-native-openai-*) ;; *) exit 2;; esac
case "$tested" in /srv/gitea/butler-acceptance/issue115-utility-trace-openai-*) ;; *) exit 2;; esac
test ${#source} = 40
python3 - "$tested" "$source" <<'PY'
import json,pathlib,sys,hashlib
r=pathlib.Path(sys.argv[1]);g=json.loads((r/'reports/repair-summary.json').read_text())
assert g['source']==sys.argv[2] and g['controlled_slots']==72 and g['test_failures']==0 and g['test_skips']==0 and g['native_requests']==0
assert hashlib.sha256((r/'reports/tests.jsonl').read_bytes()).hexdigest()==g['tests_sha256']
assert hashlib.sha256((r/'reports/build.log').read_bytes()).hexdigest()==g['unit_and_race_build_log_sha256']
assert 'PASS' in (r/'reports/openai-tests.log').read_text() and (r/'reports/openai-tests.exit').read_text().strip()=='0'
PY
mkdir "$r" "$r/out" "$r/bin" "$r/previous" "$r/pgdata"
cp -a --reflink=auto "$tested/source" "$r/source"
for file in app.test broker extension.test http.test test2json; do cp -p "$tested/reports/$file" "$r/bin/$file"; done
cp -a "$reference/previous/." "$r/previous/"
cp -p "$reference/public-report.json" "$r/previous/gemini-v3-completed.json"
cp -p "$tested/reports/repair-summary.json" "$r/previous/openai-controlled.json"
image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
docker run --rm --network none --cpus 1 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --user 1000:1000 --log-driver none --tmpfs /tmp:rw,size=128m \
 -v "$r/source:/source:ro" -v "$r/bin:/binproof:ro" -v "$r/out:/out" -v "$r/previous:/previous:ro" -v "$reference/out/plan:/reference:ro" \
 -e BUTLER_UTILITY_OPENAI=gpt-6-luna -e BUTLER_UTILITY_NATIVE_FREEZE=/out/plan -e BUTLER_UTILITY_NATIVE_SOURCE="$source" -w /source/apps/orchestrator/internal/app \
 "$image" /binproof/app.test -test.v -test.run '^TestMemoryUtilityFreezeNativeABC$' > "$r/freeze.log" 2>&1
python3 - "$r" "$reference" <<'PY'
import json,pathlib,sys,hashlib
r,old=map(pathlib.Path,sys.argv[1:]);plan=json.loads((r/'out/plan/plan.json').read_text());prior=json.loads((old/'out/plan/plan.json').read_text())
assert plan['Provider']=='openai' and plan['Model']=='gpt-6-luna' and plan['ReasoningEffort']=='high'
assert plan['Slots']==prior['Slots'] and len(plan['Slots'])==72 and plan['SuiteSHA256']==prior['SuiteSHA256']
assert (r/'out/plan/suite.private.json').read_bytes()==(old/'out/plan/suite.private.json').read_bytes()
assert plan['ReferenceSuiteSHA256']==hashlib.sha256((old/'out/plan/suite.private.json').read_bytes()).hexdigest()
assert 'gemini-v3-completed.json' in plan['HistoricalReceipts']
PY
pg=$(basename "$r")-pg
helper=$(basename "$r")-runner
made_pg=0; cid=
net=$(basename "$r")-net
made_net=0
cleanup(){
 status=$?;trap - EXIT
 if [ -n "$cid" ]; then docker inspect "$cid" > "$r/final-container.json" 2>/dev/null || true;docker rm -f "$cid" >/dev/null;fi
 if [ "$made_pg" = 1 ]; then docker stop -t 15 "$pg" > "$r/pg-stop.txt";fi
 if [ "$made_net" = 1 ]; then docker network disconnect "$net" "$pg";docker network rm "$net" > "$r/network-removed.txt";fi
 printf '{"exit_code":%s,"provider":"openai","model":"gpt-6-luna","previous_results_preserved":true,"persistent_api_key_copies":0}\n' "$status" > "$r/exit.json"
 exit "$status"
}
trap cleanup EXIT
docker network create --subnet 10.249.120.0/28 "$net" > "$r/network.id";made_net=1
docker run -d --name "$pg" --network "$net" --cpus 1 --memory 768m --memory-swap 768m --pids-limit 128 -p 127.0.0.1:55456:5432 \
 -e POSTGRES_USER=utility_fixture -e POSTGRES_PASSWORD=utility_fixture_only -e POSTGRES_DB=utility_native_test \
 -v "$r/pgdata:/var/lib/postgresql/data" sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d > "$r/pg.id"
made_pg=1
for n in $(seq 1 40); do if docker exec "$pg" pg_isready -U utility_fixture -d utility_native_test >/dev/null 2>&1; then break;fi;sleep 1;done
cid=$(docker create --name "$helper" --network host --cpus 1 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --user 1000:1000 --log-driver none --tmpfs /tmp:rw,nosuid,nodev,size=512m \
 -v "$r/source:/source:ro" -v "$r/bin:/binproof:ro" -v "$r/out:/out" -v "$r/out/plan:/plan:ro" -v "$r/previous:/previous:ro" \
 -e HTTPS_PROXY=http://127.0.0.1:18080 -e NO_PROXY=127.0.0.1,localhost,external.test -e BUTLER_UTILITY_NATIVE=utility-abc-20261030-v1 -e BUTLER_UTILITY_NATIVE_SOURCE="$source" \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://utility_fixture:utility_fixture_only@127.0.0.1:55456/utility_native_test?sslmode=disable' \
 -e BUTLER_BROKER_E2E_BINARY=/binproof/broker -e BUTLER_EXTENSION_E2E_BINARY=/binproof/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/binproof/http.test -e GOMAXPROCS=1 \
 -w /source/apps/orchestrator/internal/app "$image" /bin/sleep 15000)
docker start "$cid" >/dev/null
docker inspect "$cid" > "$r/container-before-auth.json"
python3 - "$r" "$source" <<'PY'
import sys,json,pathlib,hashlib
r=pathlib.Path(sys.argv[1]);d=json.loads((r/'container-before-auth.json').read_text())[0];h=d['HostConfig'];p=d['State']['Pid']
assert d['State']['Running'] and d['Config']['User']=='1000:1000' and not h['Privileged'] and h['ReadonlyRootfs'] and h['NetworkMode']=='host' and h['PidMode']==''
assert h['NanoCpus']==1000000000 and h['Memory']==3221225472 and h['MemorySwap']==3221225472 and h['PidsLimit']==256 and 'ALL' in h['CapDrop']
assert any(x.startswith('no-new-privileges') for x in h['SecurityOpt'])
mounts={x['Destination']:x for x in d['Mounts']};assert set(mounts)=={'/source','/binproof','/out','/plan','/previous'} and all(not x['RW'] for k,x in mounts.items() if k!='/out')
assert not any(x.startswith(('OPENAI_API_KEY=','BUTLER_OPENAI_API_KEY=')) for x in d['Config']['Env'])
status=dict(x.split(':',1) for x in pathlib.Path(f'/proc/{p}/status').read_text().splitlines() if ':' in x)
assert int(status['CapEff'].strip(),16)==0 and status['NoNewPrivs'].strip()=='1' and status['Seccomp'].strip()=='2'
assert pathlib.Path(f'/proc/{p}/ns/pid').readlink()!=pathlib.Path('/proc/self/ns/pid').readlink()
cg=next(x.split('::',1)[1] for x in pathlib.Path(f'/proc/{p}/cgroup').read_text().splitlines() if x.startswith('0::'));group=pathlib.Path('/sys/fs/cgroup')/cg.lstrip('/');assert d['Id'] in str(group)
v={k:(group/k).read_text().strip() for k in ['cpu.max','memory.max','memory.swap.max','pids.max']};assert v=={'cpu.max':'100000 100000','memory.max':'3221225472','memory.swap.max':'0','pids.max':'256'}
proof={'source':sys.argv[2],'pid':p,'cgroup':str(group),'actual_kernel_values':v,'binary_sha256':hashlib.sha256((r/'bin/app.test').read_bytes()).hexdigest(),'plan_sha256':hashlib.sha256((r/'out/plan/plan.json').read_bytes()).hexdigest(),'native_requests_before_proof':0,'key_stdin_only':True}
(r/'out/kernel-verified.json').write_text(json.dumps(proof)+'\n')
PY
set +e
docker exec -i "$cid" /binproof/app.test -test.v -test.timeout=14500s -test.run '^TestMemoryUtilityWholeNativeABC$' > "$r/native.log" 2>&1
native_status=$?
set -e
docker exec -i "$cid" /binproof/test2json -t -p github.com/butler/butler/apps/orchestrator/internal/app < "$r/native.log" > "$r/native.jsonl"
journal_schema=$(python3 -c 'import json,sys;print("utility_abc_"+json.load(open(sys.argv[1]))["SuiteSHA256"][:16])' "$r/out/plan/plan.json")
docker exec "$pg" psql -U utility_fixture -d utility_native_test -Atc "SELECT COALESCE(jsonb_agg(to_jsonb(e) ORDER BY seq),'[]') FROM $journal_schema.entries e" > "$r/out/journal-complete.private.json"
exit "$native_status"

#!/bin/sh
set -eu
umask 077
r=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
p=$r/skill-broker-20261029-v1
pg=issue115-native-pg-20261028-v1
image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
source=605f6cb37077c3857ff3caae2220f0097d899e35
app=b0c96190a6f7ff5b2cea966b2a2b3f78c670cb77137fc75c97a261d32c1e97ee
broker=fa1396b6bd9218ab3c25c574e99cd196feeebaabe48faa4d4685d3d96f9cac0c
extension=40c25e3fa429763f713580eff0b967719b72e66d23eb77a39b4ef81dea4b473b
test ! -e "$p/native-started.txt"
test ! -e "$p/source"
grep -q '"exit_code":0' "$p/safety-exit.json"
test "$(sha256sum "$p/source.tar" | cut -d ' ' -f1)" = 447436cb971f8f0bcfea3cd1c328e1dd78878d008f9f88f8a55e784d81d21c3e
test "$(sha256sum "$p/bin/app.test" | cut -d ' ' -f1)" = "$app"
test "$(sha256sum "$p/bin/tool-broker" | cut -d ' ' -f1)" = "$broker"
test "$(sha256sum "$p/bin/extension.test" | cut -d ' ' -f1)" = "$extension"
test "$(sha256sum "$p/out/plan.json" | cut -d ' ' -f1)" = c511fc06b1dbcfeb9e62ca2f366d20aa077a708d0c42484ac379bf23619a2442
mkdir "$p/source"
tar -xf "$p/source.tar" -C "$p/source"
for f in apps/orchestrator/internal/orchestrator/skill_context.go apps/orchestrator/internal/orchestrator/catalog_recovery.go apps/orchestrator/internal/orchestrator/skill_context_test.go apps/orchestrator/internal/orchestrator/skill_selection_context_test.go apps/orchestrator/internal/app/skills_e2e_test.go apps/orchestrator/internal/app/skill_broker_native_test.go; do
  cmp "$p/source/$f" "$p/delta/$(basename "$f")"
done
chmod -R a-w "$p/source"
test "$(docker inspect -f '{{.Id}}' "$pg")" = "$(cat "$r/native-pg-container.txt")"
test "$(docker inspect -f '{{.State.Running}}' "$pg")" = false
date -u +%Y-%m-%dT%H:%M:%SZ > "$p/native-started.txt"
docker start "$pg" > /dev/null
cid=
cleanup() {
  if test -n "$cid"; then
    docker top "$cid" -eo pid > "$p/remaining-processes.txt" 2>/dev/null || true
    docker inspect "$cid" > "$p/container-final.json" 2>/dev/null || true
    docker rm -f "$cid" > /dev/null
  fi
  clients=$(docker exec "$pg" psql -U postgres -p 55451 -d behavior76_test -Atc "SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()")
  if test "$clients" = 0; then docker stop -t 15 "$pg" > /dev/null; fi
  python3 - "$p" "$clients" <<'PY'
import json,pathlib,sys,time
p=pathlib.Path(sys.argv[1]); proof=json.loads((p/'out/kernel-verified.json').read_text()) if (p/'out/kernel-verified.json').exists() else None
remaining=(p/'remaining-processes.txt').read_text().split() if (p/'remaining-processes.txt').exists() else []
only_init=bool(proof) and remaining==['PID',str(proof['pid'])]
empty=False
if proof:
    group=pathlib.Path(proof['cgroup'])
    for _ in range(20):
        try: empty='populated 0' in (group/'cgroup.events').read_text()
        except FileNotFoundError: empty=True
        if empty: break
        time.sleep(.1)
result={'remaining_clients':int(sys.argv[2]),'private_postgres_stopped':sys.argv[2]=='0','probe_container_removed':True,'only_helper_init_after_test':only_init,'whole_cgroup_empty':empty,'forced_cleanup_of_child':not only_init,'qualified_exit':only_init and empty and sys.argv[2]=='0'}
(p/'cleanup.json').write_text(json.dumps(result)+'\n')
print(json.dumps(result))
if not result['qualified_exit']:sys.exit(1)
PY
}
trap cleanup EXIT
for n in 1 2 3 4 5; do
  if docker exec "$pg" pg_isready -U postgres -p 55451 > /dev/null; then break; fi
  sleep 1
done
cid=$(docker create --name skill-broker-native-20261029-v1 --network host --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=128m --mount type=bind,src="$p/source",dst=/source,readonly --mount type=bind,src="$p/bin",dst=/binproof,readonly --mount type=bind,src="$p/out",dst=/out --mount type=bind,src="$p/out/plan.json",dst=/plan.json,readonly --mount type=bind,src="$p/previous",dst=/previous,readonly --mount type=bind,src="$r/auth-source/settings.key",dst=/private/settings.key,readonly -e HTTPS_PROXY=http://127.0.0.1:18080 -e BUTLER_SKILL_BROKER_NATIVE=skill-broker-20261029-v1 -e BUTLER_SKILL_BROKER_SOURCE="$source" -e BUTLER_EXTENSION_E2E_BINARY=/binproof/extension.test -e BUTLER_BROKER_E2E_BINARY=/binproof/tool-broker -e GOMAXPROCS=1 -w /source "$image" /bin/sleep 1200)
docker start "$cid" > /dev/null
docker inspect "$cid" > "$p/container-before-auth.json"
python3 - "$p" "$image" "$app" "$source" "$broker" "$extension" <<'PY'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1]); d=json.loads((p/'container-before-auth.json').read_text())[0];h=d['HostConfig'];c=d['Config'];s=d['State']
assert d['Image']==sys.argv[2] and c['User']=='1000:1000' and s['Running']
assert not h['Privileged'] and h['NetworkMode']=='host' and h['PidMode']=='' and h['ReadonlyRootfs']
assert h['Memory']==2147483648 and h['MemorySwap']==2147483648 and h['NanoCpus']==1000000000 and h['PidsLimit']==256
assert 'ALL' in h['CapDrop'] and any(x.startswith('no-new-privileges') for x in h['SecurityOpt'])
mounts={x['Destination']:x for x in d['Mounts']};assert set(mounts)=={'/source','/binproof','/out','/plan.json','/previous','/private/settings.key'}
assert all(not x['RW'] for k,x in mounts.items() if k!='/out')
pid=s['Pid'];assert pid>0 and pathlib.Path(f'/proc/{pid}/ns/pid').readlink()!=pathlib.Path('/proc/self/ns/pid').readlink()
status=dict(x.split(':',1) for x in pathlib.Path(f'/proc/{pid}/status').read_text().splitlines() if ':' in x)
assert status['Uid'].split()==['1000']*4 and int(status['CapEff'].strip(),16)==0 and status['NoNewPrivs'].strip()=='1' and status['Seccomp'].strip()=='2'
cg=next(x.split('::',1)[1] for x in pathlib.Path(f'/proc/{pid}/cgroup').read_text().splitlines() if x.startswith('0::'))
group=pathlib.Path('/sys/fs/cgroup')/cg.lstrip('/');assert d['Id'] in str(group)
values={name:(group/name).read_text().strip() for name in ['cpu.max','memory.max','memory.swap.max','pids.max']}
assert values=={'cpu.max':'100000 100000','memory.max':'2147483648','memory.swap.max':'0','pids.max':'256'}
proof={'binary_sha256':sys.argv[3],'source':sys.argv[4],'broker_sha256':sys.argv[5],'extension_sha256':sys.argv[6],'container_id':d['Id'],'pid':pid,'cgroup':str(group),'actual_kernel_values':values,'private_pid_namespace':True,'generation_requests_before_proof':0}
(p/'out/kernel-verified.json').write_text(json.dumps(proof)+'\n')
PY
set +e
docker exec "$cid" /binproof/app.test -test.run='^TestReviewedSkillNativeBrokerPipeline$' -test.v -test.timeout=960s > "$p/native.log" 2>&1
status=$?
set -e
printf '{"exit_code":%s,"maximum_new_requests":12,"source":"%s","app_sha256":"%s","broker_sha256":"%s","extension_sha256":"%s"}\n' "$status" "$source" "$app" "$broker" "$extension" > "$p/native-exit.json"
date -u +%Y-%m-%dT%H:%M:%SZ > "$p/native-finished.txt"
tail -n 12 "$p/native.log"
exit "$status"

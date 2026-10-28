#!/bin/sh
set -eu
umask 077
r=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
p=$r/skill-prompt-20261029-v1
pg=issue115-native-pg-20261028-v1
image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
source=09b4c563da5998ac9baccde44d0405244b08032b
binary=580f1307bc3f0f73d12faaa30fc37413dea868fe173f4ee3fdab8842e8e5fd30
test ! -e "$p/native-started.txt"
test "$(sha256sum "$p/out/probe.test" | cut -d ' ' -f 1)" = "$binary"
test "$(docker inspect -f '{{.Id}}' "$pg")" = "$(cat "$r/native-pg-container.txt")"
test "$(docker inspect -f '{{.State.Running}}' "$pg")" = false
date -u +%Y-%m-%dT%H:%M:%SZ > "$p/native-started.txt"
docker start "$pg" > /dev/null
cid=
cleanup() {
  if test -n "$cid"; then
    docker inspect "$cid" > "$p/container-final.json" 2>/dev/null || true
    docker rm -f "$cid" > /dev/null
  fi
  clients=$(docker exec "$pg" psql -U postgres -p 55451 -d behavior76_test -Atc "SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()")
  if test "$clients" = 0; then
    docker stop -t 15 "$pg" > /dev/null
    printf '%s\n' '{"remaining_clients":0,"probe_container_removed":true,"private_postgres_stopped":true}' > "$p/cleanup.json"
  else
    printf '{"remaining_clients":%s,"private_postgres_stopped":false}\n' "$clients" > "$p/cleanup.json"
  fi
}
trap cleanup EXIT
for n in 1 2 3 4 5; do
  if docker exec "$pg" pg_isready -U postgres -p 55451 -d behavior76_test > /dev/null; then break; fi
  sleep 1
done
cid=$(docker create --name skill-prompt-native-20261029-v1 --network host --cpus 1 --memory 2g --memory-swap 2g --pids-limit 128 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=32m --mount type=bind,src="$p/out",dst=/out --mount type=bind,src="$r/auth-source/settings.key",dst=/private/settings.key,readonly --mount type=bind,src="$r/review-output-abc-v6/cost-review.json",dst=/previous/cost-review.json,readonly --mount type=bind,src="$p/plan.json",dst=/plan.json,readonly --mount type=bind,src="$r/skill-force-20261028-v1/out/native-result.json",dst=/previous/forced-native-result.json,readonly -e HTTPS_PROXY=http://127.0.0.1:18080 -e BUTLER_PROMPT_PROBE=skill-prompt-20261029-v1 -e BUTLER_PROMPT_SOURCE="$source" -e GOMAXPROCS=1 "$image" /bin/sleep 1200)
docker start "$cid" > /dev/null
docker inspect "$cid" > "$p/container-before-auth.json"
python3 - "$p" "$image" "$binary" "$source" <<'PY'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1]); d=json.loads((p/'container-before-auth.json').read_text())[0]
h=d['HostConfig']; c=d['Config']; s=d['State']
assert d['Image']==sys.argv[2] and c['User']=='1000:1000' and s['Running']
assert h['NetworkMode']=='host' and h['PidMode']=='' and h['ReadonlyRootfs']
assert h['Memory']==2147483648 and h['MemorySwap']==2147483648 and h['NanoCpus']==1000000000 and h['PidsLimit']==128
assert 'ALL' in h['CapDrop'] and any(x.startswith('no-new-privileges') for x in h['SecurityOpt'])
mounts={x['Destination']:x for x in d['Mounts']}
assert set(mounts)=={'/out','/private/settings.key','/previous/cost-review.json','/plan.json','/previous/forced-native-result.json'}
assert not mounts['/plan.json']['RW'] and not mounts['/previous/forced-native-result.json']['RW']
assert not mounts['/private/settings.key']['RW'] and not mounts['/previous/cost-review.json']['RW']
pid=s['Pid']; assert pid>0
assert pathlib.Path(f'/proc/{pid}/ns/pid').readlink()!=pathlib.Path('/proc/self/ns/pid').readlink()
proof={'binary_sha256':sys.argv[3],'source':sys.argv[4],'container_id':d['Id'],'pid':pid,'kernel_limits_verified':True,'generation_requests_before_proof':0}
(p/'out/kernel-verified.json').write_text(json.dumps(proof)+'\n')
PY
set +e
docker exec "$cid" /out/probe.test -test.run='^TestSkillPromptProbeNative$' -test.v -test.timeout=930s > "$p/native.log" 2>&1
status=$?
set -e
printf '{"exit_code":%s,"maximum_new_requests":36,"source":"%s","binary_sha256":"%s"}\n' "$status" "$source" "$binary" > "$p/native-exit.json"
date -u +%Y-%m-%dT%H:%M:%SZ > "$p/native-finished.txt"
cat "$p/native.log"
test -f "$p/out/result.json" && cat "$p/out/result.json"
exit "$status"

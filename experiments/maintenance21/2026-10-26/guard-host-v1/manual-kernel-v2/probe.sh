#!/bin/sh
set -eu
cd /src21
export DOCKER_HOST=unix:///run/butler21-docker.sock DOCKER_CONFIG=/src21/.cache/docker-private21
out=/src21/.cache/maintenance21-guard-kernel-v2
image=5a7e4124365ea4f271c23e1c9c918b991f86d675e0f2789e520f87a321062165
name=butler21-guard-probe-v2
if docker container inspect "$name" >/dev/null 2>&1;then exit 2;fi
test -z "$(docker ps -q)"
test "$(cat /sys/fs/cgroup/butler-maintenance21/memory.swap.max)" = 0
id=''
cleanup() {
 if test -n "$id";then
  docker kill --signal=KILL "$id" > "$out/emergency-kill.log" 2>&1 || true
  docker wait "$id" > "$out/emergency-wait.log" 2>&1 || true
  docker rm "$id" > "$out/emergency-remove.log" 2>&1 || true
 fi
}
trap cleanup EXIT
id=$(docker create --name "$name" --user 65532:65532 --hostname maintenance-capsule --workdir / --entrypoint /maintenance-guard --env GOMAXPROCS=2 --env PATH=/ --env HOME=/ --env HOSTNAME=maintenance-capsule --no-healthcheck --read-only --network none --ipc private --cgroupns private --cap-drop ALL --security-opt no-new-privileges:true --pids-limit 32 --cpus 1 --memory 128m --memory-swap 128m --tmpfs '/control:rw,nosuid,nodev,noexec,size=65536,uid=0,gid=0,mode=0755' --log-driver none --ulimit nofile=64:64 "sha256:$image")
case "$id" in *[!0-9a-f]*|'') exit 2;;esac
test "${#id}" = 64
printf '%s\n' "$id" > "$out/container-id.txt"
docker start "$id" > "$out/start.log"
pid=$(docker inspect "$id" --format '{{.State.Pid}}')
case "$pid" in *[!0-9]*|'') exit 2;;esac
test "$pid" -gt 1
printf 'pid=%s\n' "$pid"
test "$(docker inspect "$id" --format '{{.Image}}')" = "sha256:$image"
test "$(sha256sum "/proc/$pid/exe" | cut -d ' ' -f 1)" = c62270e6ca372fa6062f41247bdcfcb9a34d3649cf7df0e58a3119ed16add1c9
test ! -e "/proc/$pid/root/control/launch.json"
printf 'trusted_guard_hash_match=true\nmarker_absent=true\ncandidate_launched=false\n'
awk '/^(Uid|Gid|Groups|NSpid|CapInh|CapPrm|CapEff|CapBnd|CapAmb|NoNewPrivs|Seccomp):/{print}' "/proc/$pid/status"
group=$(cut -d : -f 3 "/proc/$pid/cgroup")
test "$(basename "$group")" = "$id"
for field in memory.max pids.max cpu.max;do printf '%s=%s\n' "$field" "$(cat "/sys/fs/cgroup$group/$field")";done
printf 'effective_ancestor_swap=%s\n' "$(cat /sys/fs/cgroup/butler-maintenance21/memory.swap.max)"
awk '{for(i=1;i<=NF;i++){if($i=="-"){print $5 "|" $(i+1) "|" $6;break}}}' "/proc/$pid/mountinfo" > "$out/mount-shape.txt"
cat "$out/mount-shape.txt"
for ns in pid mnt net;do test "$(readlink "/proc/$pid/ns/$ns")" != "$(readlink "/proc/self/ns/$ns")";printf 'private_%s_namespace=true\n' "$ns";done
actual_env=$(tr '\000' '\n' < "/proc/$pid/environ" | sort)
expected_env=$(printf 'GOMAXPROCS=2\nHOME=/\nHOSTNAME=maintenance-capsule\nPATH=/\n' | sort)
test "$actual_env" = "$expected_env"
printf 'fixed_environment_match=true\n'
cat "/proc/$pid/net/route" > "$out/ipv4-routes.txt"
cat "/proc/$pid/net/ipv6_route" > "$out/ipv6-routes.txt"
cat "/proc/$pid/net/dev" > "$out/network-interfaces.txt"
stat -c 'control_owner=%u:%g control_mode=%a' "/proc/$pid/root/control"
for file in candidate maintenance-guard;do stat -c '%n mode=%a owner=%u:%g size=%s' "/proc/$pid/root/$file";done
docker kill --signal=KILL "$id" > "$out/kill.log"
docker wait "$id" > "$out/wait.log"
test "$(docker inspect "$id" --format '{{.State.Running}}:{{.State.Pid}}')" = false:0
docker rm "$id" > "$out/remove.log"
id=''
trap - EXIT
printf 'known_container_killed_waited_removed=true\nno_marker_published=true\n'

#!/bin/sh
set -eu
umask 077
environment=/mnt/host/wsl/butler-effort76
image=/mnt/host/d/CodexData/private/butler-21-engine-20261026/engine-data-v1.ext4
storage=/mnt/host/wsl/butler-maintenance21-data
source=/mnt/host/d/CodexData/home/worktrees/issue21-local/butler
test "$(readlink -f "$environment")" = "$environment"
test "$(readlink -f "$image")" = "$image"
test "$(readlink -f "$source")" = "$source"
test -f "$image"
test "$(stat -c %s "$image")" = 8589934592
if test -f "$environment/run/butler21-docker.pid";then
 pid=$(cat "$environment/run/butler21-docker.pid")
 case "$pid" in ''|*[!0-9]*) exit 2;;esac
 test ! -d "/proc/$pid"
 test ! -e "$environment/run/butler21-docker.pid.closed-resume-v3"
 mv "$environment/run/butler21-docker.pid" "$environment/run/butler21-docker.pid.closed-resume-v3"
fi
test ! -e "$storage"
mkdir "$storage"
mount -t ext4 -o loop,noatime "$image" "$storage"
test -d "$storage/lost+found"
for directory in docker21-data src21 sys;do
 test "$(readlink -f "$environment/$directory")" = "$environment/$directory"
 test -d "$environment/$directory"
 ! mountpoint -q "$environment/$directory"
done
mount --bind "$storage" "$environment/docker21-data"
mount --bind "$source" "$environment/src21"
mount --rbind /sys "$environment/sys"
test ! -e /sys/fs/cgroup/butler-maintenance21
mkdir /sys/fs/cgroup/butler-maintenance21
printf '0\n' > /sys/fs/cgroup/butler-maintenance21/memory.swap.max
printf '2147483648\n' > /sys/fs/cgroup/butler-maintenance21/memory.max
printf '256\n' > /sys/fs/cgroup/butler-maintenance21/pids.max
printf '+cpu +memory +pids\n' > /sys/fs/cgroup/butler-maintenance21/cgroup.subtree_control
test "$(cat /sys/fs/cgroup/butler-maintenance21/memory.swap.max)" = 0
exec /bin/sh "$environment/src21/.cache/start-private-dockerd21-v2.sh"

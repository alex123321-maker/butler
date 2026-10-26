#!/bin/sh
set -eu
root=/sys/fs/cgroup
test "$(cat /proc/self/cgroup)" = '0::/'
printf 'available: '; cat "$root/cgroup.controllers"
printf 'before: '; cat "$root/cgroup.subtree_control"
for required in cpu memory pids; do
 case " $(cat "$root/cgroup.controllers") " in *" $required "*) ;; *) exit 3 ;; esac
done
printf '+cpu +memory +pids' > "$root/cgroup.subtree_control"
printf 'after: '; cat "$root/cgroup.subtree_control"
printf 'No resource limit was applied to an existing process or cgroup.\n'

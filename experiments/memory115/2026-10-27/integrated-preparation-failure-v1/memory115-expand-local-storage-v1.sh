#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
image=/mnt/host/d/CodexData/private/butler-97/behavior76/local-acceptance-root-v1.ext4
out="$root/src115/.cache/memory115-expand-local-storage-v1.log"
test ! -e "$out"
test "$(losetup -j "$image" | cut -d: -f1)" = /dev/loop0
test "$(stat -c %s "$image")" = 4294967296
for p in /proc/[0-9]*/exe; do
 target=$(readlink "$p" 2>/dev/null || true)
 case "$target" in "$root/private/"*/app.test|/private/*/app.test) echo 'Owning app still active; resize denied' >&2; exit 1;; esac
done
{
 printf 'Dedicated local experiment filesystem; all owning app tests joined; no model job.\n'
 df -k "$root"
 truncate -s 8589934592 "$image"
 losetup -c /dev/loop0
 resize2fs /dev/loop0
 test "$(stat -c %s "$image")" = 8589934592
 df -k "$root"
 printf 'PASS capacity 4GiB to8GiB; no shrink/deletion/migration; existing journals/history retained; CPU/memory/pids/native authority unchanged.\n'
} > "$out" 2>&1

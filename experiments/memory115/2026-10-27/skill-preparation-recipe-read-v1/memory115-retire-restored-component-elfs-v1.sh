#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
archive="$root/src115/.cache/memory115/prepared-artifact-offload-v1"
temporary="$root/private/memory115-prepared-20261026-v1/bin"
test "$(readlink -f "$temporary")" = "$root/private/memory115-prepared-20261026-v1/bin"
test -s "$root/src115/.cache/memory115-skill-successor-read-v1-exit.json"
test -s "$root/private/memory115-prepared-skillprep-v1/binaries.sha256"
test ! -e "$root/src115/.cache/memory115-retire-restored-component-elfs-v1.log"
for p in /proc/[0-9]*/exe; do
 target=$(readlink "$p" 2>/dev/null || true)
 case "$target" in "$temporary/"*|/private/memory115-prepared-20261026-v1/bin/*) echo 'Old artifact still owns a process; nothing removed' >&2; exit 1;; esac
done
for name in app.test broker-control.test extension.test http.test tool-broker; do
 case "$name" in
 app.test) expected=6fbebc45b0d68c723450941a155b0642821904270f67862b1238734c4112795f;;
 broker-control.test) expected=1791ba611b4ea9ac45837bac710f1a9a7cbe27ae7d9eed444bf3a0c1e2721b98;;
 extension.test) expected=2a409ae52762d3468e5d0693f80b770f455b284a11156e7ada22e9f0b586d106;;
 http.test) expected=ba5d56a18162da93bcd57c228dcca0f421f479e61fc95aa24c40c71547217ee4;;
 tool-broker) expected=f49e585b57187791e77cb7c39e4b9cab239f5d5a3c751f8d348e88329e180c3d;;
 esac
 test -f "$archive/$name" && test ! -L "$archive/$name"
 test -f "$temporary/$name" && test ! -L "$temporary/$name"
 test "$(sha256sum "$archive/$name" | cut -d' ' -f1)" = "$expected"
 test "$(sha256sum "$temporary/$name" | cut -d' ' -f1)" = "$expected"
done
for name in app.test broker-control.test extension.test http.test tool-broker; do
 rm -- "$temporary/$name"
 printf 'Temporary restored component copy retired; archive retained: %s\n' "$name"
done > "$root/src115/.cache/memory115-retire-restored-component-elfs-v1.log"
printf 'All five exact archived files remain; no source/data/auth/history/old native domain changed.\n' >> "$root/src115/.cache/memory115-retire-restored-component-elfs-v1.log"

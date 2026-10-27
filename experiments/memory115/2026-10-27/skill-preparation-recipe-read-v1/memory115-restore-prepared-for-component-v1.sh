#!/bin/sh
set -eu
umask 077
root=/mnt/host/wsl/butler-effort76
source="$root/src115/.cache/memory115/prepared-artifact-offload-v1"
destination="$root/private/memory115-prepared-20261026-v1/bin"
test "$(readlink -f "$destination")" = "$root/private/memory115-prepared-20261026-v1/bin"
for name in app.test broker-control.test extension.test http.test tool-broker; do test ! -e "$destination/$name"; done
for name in app.test broker-control.test extension.test http.test tool-broker; do
 case "$name" in
 app.test) expected=6fbebc45b0d68c723450941a155b0642821904270f67862b1238734c4112795f;;
 broker-control.test) expected=1791ba611b4ea9ac45837bac710f1a9a7cbe27ae7d9eed444bf3a0c1e2721b98;;
 extension.test) expected=2a409ae52762d3468e5d0693f80b770f455b284a11156e7ada22e9f0b586d106;;
 http.test) expected=ba5d56a18162da93bcd57c228dcca0f421f479e61fc95aa24c40c71547217ee4;;
 tool-broker) expected=f49e585b57187791e77cb7c39e4b9cab239f5d5a3c751f8d348e88329e180c3d;;
 esac
 test -f "$source/$name" && test ! -L "$source/$name"
 got=$(sha256sum "$source/$name"); test "${got%% *}" = "$expected"
 cp -p "$source/$name" "$destination/$name"
 chmod 0555 "$destination/$name"
 sync "$destination/$name"
 got=$(sha256sum "$destination/$name"); test "${got%% *}" = "$expected"
 printf 'EXACT_HISTORICAL_ELF_RESTORED %s %s ' "$name" "$expected"
 wc -c <"$destination/$name"
done
printf 'Archive retained; no auth, data, provider or old native identity opened. For controlled component gate only; new native cohort must use fresh five artifacts.\n'

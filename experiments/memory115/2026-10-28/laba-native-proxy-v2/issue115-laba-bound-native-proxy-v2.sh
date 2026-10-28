#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cd "$base"
test ! -e bound-native-proxy-v2.log
test ! -e operator-native-proxy-v2
grep -q '"exit_code":0' bound-prepare-proxy-v2-exit.json
grep -q SERVER_PRESEAL_KERNEL_AND_REAL_SOURCE_FENCES_PASS operator-prepare-proxy-v2/prepared.json
test "$(sha256sum operator-bin-proxy-v2/memory115-laba-execute | cut -d ' ' -f 1)" = 47e34f6b64d789051ee7b8fa6c19c7e164fb07e261e287b4863a35a16777bd21
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
test "$(d inspect -f '{{.Id}}' issue115-proxy-relay-20261028-v2)" = "$(cat proxy-relay-v2.cid)"
test "$(d inspect -f '{{.State.Running}}' issue115-proxy-relay-20261028-v2)" = true
date -u +%Y-%m-%dT%H:%M:%SZ > bound-native-proxy-v2-started.txt
set +e
env -i PATH=/usr/bin:/bin HOME=/home/laba HTTPS_PROXY=http://127.0.0.1:18080 ./operator-bin-proxy-v2/memory115-laba-execute native > bound-native-proxy-v2.log 2>&1
status=$?
printf '{"exit_code":%s,"operator_sha256":"47e34f6b64d789051ee7b8fa6c19c7e164fb07e261e287b4863a35a16777bd21","collector_producer":"683280b21e7c21f5ad4a87ca63d9e5e3491e0e3d","plan_sha256":"d366ab3b8c10518cf4bfda8ef627e2ea50101847a159a9afbf8ed1a53433d9a7"}\n' "$status" > bound-native-proxy-v2-exit.json
date -u +%Y-%m-%dT%H:%M:%SZ > bound-native-proxy-v2-finished.txt
exit "$status"

#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
mode=${1:?mode required}
nonce=${2:?host source marker required}
test -f "$root/etc/alpine-release"
test "$(cat "$root/src20/.cache/behavior76/host-marker")" = "$nonce"
source=${3:-unused}
output=${4:-unused}
exec chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOMAXPROCS=4 BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable' BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-v2/stimuli.jsonl "BUTLER_BEHAVIOR76_SOURCE=$source" "BUTLER_BEHAVIOR76_OUTPUT=/src20/$output" /bin/sh /src20/scripts/dev/behavior-local-run.sh "$mode"

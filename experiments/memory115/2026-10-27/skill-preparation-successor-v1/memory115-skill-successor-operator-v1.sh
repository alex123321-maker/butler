#!/bin/sh
# Preparation only. Do not invoke until the measured issue76 process is joined.
set -eu
umask 077
test "${1-}" = --own-native-lane-free
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test -f "$out/issue76-literal-r1-parent-joined-v1.json"
test ! -e "$out/memory115-skill-successor-operator-v1.log"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 BUTLER_TEST_OPERATOR_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_operator_test?sslmode=disable' /bin/sh -c 'cd /src115; exec go test -race -p 1 -count=1 -v -timeout=3m scripts/dev/memory115-local-execute.go scripts/dev/memory115-local-execute_test.go scripts/dev/memory115-local-execute_integration_test.go' >"$out/memory115-skill-successor-operator-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"real_credentials":0,"native_calls":0,"actual_dedicated_operator_PG":true,"release_acceptance":false}\n' "$status" >"$out/memory115-skill-successor-operator-v1-exit.json"
exit "$status"

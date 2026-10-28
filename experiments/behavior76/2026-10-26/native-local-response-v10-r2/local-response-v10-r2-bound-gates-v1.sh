#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src20/.cache/behavior76"
mode=${1:?mode}
case "$mode" in
 build)
  test ! -e "$out/local-response-v10-r2-bound-build-v1.log"
  chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly' GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src20; exec go test -race -c -o .cache/behavior76/collector-v10-r2-bound.test ./apps/orchestrator/internal/app' >"$out/local-response-v10-r2-bound-build-v1.log" 2>&1
  sha256sum "$out/collector-v10-r2-bound.test" >"$out/local-response-v10-r2-bound-elf-v1.txt"
  cp "$out/collector-v10-r2-bound.test" "$out/collector.test"
  exit 0 ;;
 setup) selected=daypreflight ;;
 composition) selected=daycomposition ;;
 *) exit 2 ;;
esac
log="$out/local-response-v10-r2-$mode-v1.log"
test ! -e "$log"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOMAXPROCS=2 BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable' BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker /bin/sh /src20/scripts/dev/behavior-local-run.sh "$selected" >"$log" 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"auth_access":0}\n' "$status" >"$out/local-response-v10-r2-$mode-v1-exit.json"
exit "$status"

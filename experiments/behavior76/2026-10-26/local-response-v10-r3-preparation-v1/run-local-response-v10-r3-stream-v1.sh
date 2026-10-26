#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src20/.cache/behavior76"
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly' GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src20; exec go test -race -c -o .cache/behavior76/collector-v10-r3-unbound.test ./apps/orchestrator/internal/app' >"$out/local-response-v10-r3-build-v1.log" 2>&1
sha256sum "$out/collector-v10-r3-unbound.test" >"$out/local-response-v10-r3-unbound-elf-v1.txt"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOMAXPROCS=2 BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable' BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only /bin/sh -c 'cd /src20; exec .cache/behavior76/collector-v10-r3-unbound.test -test.v -test.run "^TestBehavior76AuthorityStream.*$" -test.count=1 -test.timeout=6m' >"$out/local-response-v10-r3-stream-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"auth_access":0}\n' "$status" >"$out/local-response-v10-r3-stream-v1-exit.json"
tail -n 4 "$out/local-response-v10-r3-stream-v1.log"
exit "$status"

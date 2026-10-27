#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src20/.cache/behavior76"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly' GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src20; exec go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator' >"$out/local-response-v10-r2-vet-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"auth_access":0}\n' "$status" >"$out/local-response-v10-r2-vet-v1-exit.json"
exit "$status"

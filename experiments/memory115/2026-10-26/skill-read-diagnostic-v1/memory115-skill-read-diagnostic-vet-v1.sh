#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src115; exec go vet ./apps/orchestrator/internal/app' >"$out/memory115-skill-read-diagnostic-vet-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"auth_access":0}\n' "$status" >"$out/memory115-skill-read-diagnostic-vet-v1-exit.json"
exit "$status"

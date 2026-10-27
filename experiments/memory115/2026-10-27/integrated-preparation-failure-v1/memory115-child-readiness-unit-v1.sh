#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test ! -e "$out/memory115-child-readiness-unit-v1.log"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 GOFLAGS='-p=1 -buildvcs=false' /bin/sh -c 'cd /src115; exec go test -race -count=1 -v -run=^TestExperienceNativeChildReadinessRequiresEveryFrozenEndpoint$ ./apps/orchestrator/internal/app' > "$out/memory115-child-readiness-unit-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"PG":0,"native":0,"real_auth":0,"test_only_startup_observation":true}\n' "$status" > "$out/memory115-child-readiness-unit-v1-exit.json"
exit "$status"

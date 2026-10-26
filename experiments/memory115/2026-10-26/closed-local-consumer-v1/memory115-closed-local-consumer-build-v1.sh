#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test ! -e "$out/memory115-closed-local-consumer-build-v1.log"
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src115; exec go test -race -c -o .cache/memory115-closed-local-consumer-v1.test ./apps/orchestrator/internal/app' >"$out/memory115-closed-local-consumer-build-v1.log" 2>&1
sha256sum "$out/memory115-closed-local-consumer-v1.test" >"$out/memory115-closed-local-consumer-elf-v1.txt"

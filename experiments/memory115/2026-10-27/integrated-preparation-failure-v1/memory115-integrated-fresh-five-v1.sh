#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test ! -e "$out/memory115-integrated-fresh-five-v1.log"
test ! -e "$root/private/memory115-source-skillprep-v1"
test ! -e "$root/private/memory115-prepared-skillprep-v1"
test -s "$out/memory115-source-skillprep-baaf-v2-inventory.json"
test "$(sha256sum "$out/memory115-source-skillprep-baaf-v2-inventory.json" | cut -d' ' -f1)" = ec7404c33da138111fe1fb5d0d3738cb31a110431b244c1486d66525719727b7
mkdir "$root/private/memory115-source-skillprep-v1"
mount --bind "$out/memory115-source-skillprep-baaf-v2" "$root/private/memory115-source-skillprep-v1"
mount -o remount,bind,ro "$root/private/memory115-source-skillprep-v1"
mkdir -m 700 "$root/private/memory115-prepared-skillprep-v1"
mkdir -m 700 "$root/private/memory115-prepared-skillprep-v1/bin"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 GOFLAGS='-p=1 -buildvcs=false' /bin/sh -eu -c '
src=/private/memory115-source-skillprep-v1
proof=/private/memory115-prepared-skillprep-v1
cd "$src"
printf "producer=baaf94ef6298258d5590709cd649541ab371e93f\n"
go version
findmnt -n -o TARGET,VFS-OPTIONS "$src"
findmnt -n -o VFS-OPTIONS "$src" | grep -Eq "(^|,)ro(,|$)"
find . -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source-before.sha256"
go test -c -o "$proof/bin/broker-control.test" ./apps/tool-broker/internal/broker
go build -o "$proof/bin/tool-broker" ./apps/tool-broker
go test -c -o "$proof/bin/extension.test" ./apps/tool-extension/internal/runtime
go test -c -o "$proof/bin/http.test" ./apps/tool-http/internal/runtime
go test -race -c -o "$proof/bin/app.test" ./apps/orchestrator/internal/app
chmod 0555 "$proof/bin/"*
find . -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source-after.sha256"
cmp "$proof/source-before.sha256" "$proof/source-after.sha256"
sha256sum "$proof/bin/"* > "$proof/binaries.sha256"
stat -c "%n %s %a" "$proof/bin/"*
cat "$proof/binaries.sha256"
printf "PASS fresh-five; source exact before/after; native=0 auth=0\n"
' > "$out/memory115-integrated-fresh-five-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"source_commit":"baaf94ef6298258d5590709cd649541ab371e93f","five_fresh_artifacts":true,"native_calls":0,"real_credentials":0}\n' "$status" > "$out/memory115-integrated-fresh-five-v1-exit.json"
exit "$status"

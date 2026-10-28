#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e host-build-v1.log
set +e
docker run --rm --name issue115-host-build-20261028-v1 --network none --cpus 2 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=256m --mount type=bind,src="$base/host-preparation-v1",dst=/source,readonly --mount type=bind,src="$base/host-bin-v1",dst=/out --mount type=bind,src="$base/cache",dst=/cache --mount type=bind,src="$base/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=2 -e GOFLAGS='-p=1 -buildvcs=false' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c 'go test -race -count=1 -v memory115-laba-host.go memory115-laba-host_test.go; go vet memory115-laba-host.go; CGO_ENABLED=0 go build -o /out/memory115-laba-host memory115-laba-host.go; chmod 0555 /out/memory115-laba-host; sha256sum /out/memory115-laba-host' > host-build-v1.log 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"real_auth":0}\n' "$status" > host-build-v1-exit.json
exit "$status"

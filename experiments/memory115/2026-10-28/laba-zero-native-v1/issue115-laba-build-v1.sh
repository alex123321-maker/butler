#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test "$(cat readiness-exit.json)" = '{"exit_code":0,"host":"ssh laba","native_calls":0,"real_auth":0,"source_commit":"a58aa96936c01ee3f9de6d6601e02f8df9058653"}'
test ! -e build.log
mkdir prepared
set +e
docker run --rm --name issue115-build-20261028-v1 --network none --cpus 2 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=256m --mount type=bind,src="$base/source",dst=/source,readonly --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$base/cache",dst=/cache --mount type=bind,src="$base/build-tmp",dst=/build-tmp --mount type=bind,src="$base/prepared",dst=/prepared -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=2 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
go version
go test -c -o /prepared/broker-control.test ./apps/tool-broker/internal/broker
go build -o /prepared/tool-broker ./apps/tool-broker
go test -c -o /prepared/extension.test ./apps/tool-extension/internal/runtime
go test -c -o /prepared/http.test ./apps/tool-http/internal/runtime
go test -race -c -o /prepared/app.test ./apps/orchestrator/internal/app
chmod 0555 /prepared/*
sha256sum /prepared/*
' > build.log 2>&1
status=$?
printf '{"exit_code":%s,"host":"ssh laba","native_calls":0,"real_auth":0,"source_commit":"a58aa96936c01ee3f9de6d6601e02f8df9058653"}\n' "$status" > build-exit.json
exit "$status"

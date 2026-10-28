#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cachebase=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e review-build-v1.log
printf '%s\n' '{"Replace":{"/source/apps/orchestrator/internal/app/memory115_laba_cost_review_test.go":"/review-source/memory115_laba_cost_review_test.go"}}' > review-source-v1/overlay.json
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
set +e
d run --rm --name issue115-cost-review-build-v1 --network none --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m --mount type=bind,src="$base/source-proxy-v2",dst=/source,readonly --mount type=bind,src="$base/review-source-v1",dst=/review-source,readonly --mount type=bind,src="$base/review-bin-v1",dst=/out --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$cachebase/cache",dst=/cache --mount type=bind,src="$cachebase/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
go vet -overlay=/review-source/overlay.json ./apps/orchestrator/internal/app
go test -race -overlay=/review-source/overlay.json -c -o /out/review.test ./apps/orchestrator/internal/app
chmod 0555 /out/review.test
sha256sum /out/review.test
' > review-build-v1.log 2>&1
status=$?
printf '{"exit_code":%s,"generation_requests":0,"native_collector_unchanged":true,"post_exit_reader_only":true}\n' "$status" > review-build-v1-exit.json
tail -n 6 review-build-v1.log
exit "$status"

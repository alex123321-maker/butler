#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cachebase=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
grep -q '"exit_code":0' build-proxy-v2-exit.json
grep -q '"all_stage_exits_zero":true' qualification-all-v1-exit.json
test ! -e qualification-proxy-v2.log
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
set +e
d run --rm --name issue115-proxy-qualification-v2 --network container:issue115-native-pg-20261028-v1 --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m --mount type=bind,src="$base/source-proxy-v2",dst=/source,readonly --mount type=bind,src="$base/prepared-proxy-v2",dst=/prepared,readonly --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$cachebase/cache",dst=/cache --mount type=bind,src="$cachebase/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -e BUTLER_TEST_OPERATOR_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_operator_test?sslmode=disable' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
/prepared/app.test -test.v -test.count=1 -test.run="^(TestLaba115NetworkProfileRequiresFixedServerProxy|TestLocal115NativeNetworkProfileRejectsAmbientProxySubstitution|TestLaba115SuccessorKeepsHistoryAndRejectsCrossHostClaims|TestExperienceNativePresealPinsSourceArtifactsHistoryAndDenominator)$" -test.timeout=3m
go test -race -count=1 -v scripts/dev/memory115-laba-host.go scripts/dev/memory115-laba-auth.go scripts/dev/memory115-laba-execute.go scripts/dev/memory115-laba-host_test.go scripts/dev/memory115-laba-execute_test.go scripts/dev/memory115-local-execute_integration_test.go
go vet ./apps/orchestrator/internal/app
go vet scripts/dev/memory115-laba-host.go scripts/dev/memory115-laba-auth.go scripts/dev/memory115-laba-execute.go
go vet scripts/dev/memory115-laba-proxy.go
' > qualification-proxy-v2.log 2>&1
status=$?
set -e
(cd source-proxy-v2; sha256sum -c ../issue115-source-proxy-v2.sha256 > ../source-after-qualification-proxy-v2.log)
printf '{"exit_code":%s,"network_delta_tests":true,"unchanged_controls_producer":"92f878f825e8f1ec58bed9df5c0b48cfb47035e2","producer":"683280b21e7c21f5ad4a87ca63d9e5e3491e0e3d","generation_requests":0}\n' "$status" > qualification-proxy-v2-exit.json
tail -n 10 qualification-proxy-v2.log
exit "$status"

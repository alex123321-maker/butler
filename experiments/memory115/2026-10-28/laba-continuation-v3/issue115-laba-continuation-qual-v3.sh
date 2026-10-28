#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cachebase=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e continuation-qual-v3.log
mkdir continuation-bin-v3 history-abc-v3
cp -a closed-laba257-v1 history-abc-v3/closed-laba257
cp history/skill-read-diagnostic-receipt.json history/owner-decision.json history-abc-v3/
cp repaired-relay-owner-v2.json history-abc-v3/repaired-relay-owner.json
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
test "$(d inspect -f '{{.Id}}' issue115-native-pg-20261028-v1)" = "$(cat native-pg-container.txt)"
d start issue115-native-pg-20261028-v1 > continuation-pg-start-v3.log
set +e
d run --rm --name issue115-continuation-qual-v3 --network container:issue115-native-pg-20261028-v1 --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m --mount type=bind,src="$base/source-proxy-v2",dst=/source,readonly --mount type=bind,src="$base/continuation-delta-v3",dst=/delta,readonly --mount type=bind,src="$base/closed-laba257-v1",dst=/closure,readonly --mount type=bind,src="$base/continuation-bin-v3",dst=/out --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$cachebase/cache",dst=/cache --mount type=bind,src="$cachebase/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -e BUTLER_ISSUE115_LABA_CLOSED=verify-latest257 -e BUTLER_TEST_OPERATOR_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_operator_test?sslmode=disable' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
go test -overlay=/delta/overlay.json -race -count=1 -v -timeout=10m -run="^(TestLaba115ClosedFailedHistoryReceipt|TestLaba115ContinuationRetainsFailuresAndExactAuthority|TestLaba115NetworkProfileRequiresFixedServerProxy|TestLocal115NativeNetworkProfileRejectsAmbientProxySubstitution|TestLocal115SkillPreparationSuccessorPreservesLatestFailedPopulation|TestExperienceNativePresealPinsSourceArtifactsHistoryAndDenominator|TestClosedExperienceProductsPreserveUnknownAndRejectForgeries|TestClosedExperienceShortScopeRetainsOmissionsAndRejectsLateWork)$" ./apps/orchestrator/internal/app
go test -overlay=/delta/overlay.json -race -count=1 -v scripts/dev/memory115-laba-host.go scripts/dev/memory115-laba-auth.go scripts/dev/memory115-laba-execute.go scripts/dev/memory115-laba-host_test.go scripts/dev/memory115-laba-execute_test.go scripts/dev/memory115-local-execute_integration_test.go
go vet -overlay=/delta/overlay.json ./apps/orchestrator/internal/app
go test -overlay=/delta/overlay.json -race -c -o /out/app.test ./apps/orchestrator/internal/app
chmod 0555 /out/app.test
' > continuation-qual-v3.log 2>&1
status=$?
printf '{"exit_code":%s,"generation_requests":0,"real_auth":0,"closed_history":257,"unknown_native_retained":2}\n' "$status" > continuation-qual-v3-exit.json
tail -n 14 continuation-qual-v3.log
exit "$status"

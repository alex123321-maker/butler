#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cachebase=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e proxy-regression-started-v1
: > proxy-regression-started-v1
mkdir proxy-regression-output-v1
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
set +e
d run --rm --name issue115-proxy-regression-v1 --network none --cpus 1 --memory 512m --memory-swap 512m --pids-limit 128 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m --mount type=bind,src="$base/proxy-regression-v1",dst=/regression,readonly --mount type=bind,src="$base/proxy-regression-output-v1",dst=/out --mount type=bind,src="$cachebase/cache",dst=/cache --mount type=bind,src="$cachebase/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false' -e BUTLER_TEST_LABA_PROXY_LONG_REUSE=1 -w /tmp sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
go test -race -count=1 -v -timeout=7m /regression/fixed/relay.go /regression/fixed/relay_test.go > /out/fixed.log 2>&1 &
fixed_pid=$!
go test -race -count=1 -v -timeout=7m /regression/legacy/relay.go /regression/legacy/relay_test.go > /out/legacy.log 2>&1 &
legacy_pid=$!
set +e
wait "$fixed_pid"; fixed_status=$?
wait "$legacy_pid"; legacy_status=$?
set -e
printf "{\"fixed_exit\":%s,\"legacy_exit\":%s,\"generation_requests\":0,\"auth_access\":0,\"network\":\"none\"}\n" "$fixed_status" "$legacy_status" > /out/status.json
test "$fixed_status" = 0
test "$legacy_status" = 1
grep -q "persistent connection failed after 5m" /out/legacy.log
go vet /regression/fixed/relay.go
' > proxy-regression-wrapper-v1.log 2>&1
status=$?
printf '{"exit_code":%s,"generation_requests":0,"auth_access":0,"old_failure_required":true}\n' "$status" > proxy-regression-v1-exit.json
cat proxy-regression-output-v1/status.json
exit "$status"

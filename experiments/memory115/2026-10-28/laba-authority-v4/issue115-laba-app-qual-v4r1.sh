#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cachebase=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e app-qual-v4r1.log
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
d exec issue115-native-pg-20261028-v1 createdb -p55451 -Upostgres memory115_app_qual_v4r1_test
set +e
d run --rm --name issue115-app-qual-v4r1 --network container:issue115-native-pg-20261028-v1 --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m --mount type=bind,src="$base/source-abc-v3",dst=/source,readonly --mount type=bind,src="$base/full-delta-v4",dst=/delta,readonly --mount type=bind,src="$base/prepared-abc-v3",dst=/prepared,readonly --mount type=bind,src="$base/qualification-plans",dst=/plans,readonly --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$cachebase/cache",dst=/cache --mount type=bind,src="$cachebase/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_app_qual_v4r1_test?sslmode=disable' -e BUTLER_BROKER_E2E_BINARY=/prepared/tool-broker -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/prepared/broker-control.test -e BUTLER_EXTENSION_E2E_BINARY=/prepared/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/prepared/http.test -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
go test -race -overlay=/delta/overlay.json -count=1 -v -timeout=15m -run="$(cat /plans/issue115-laba-app-regex-v1.txt)" ./apps/orchestrator/internal/app
go vet -overlay=/delta/overlay.json ./apps/orchestrator/internal/app
' > app-qual-v4r1.log 2>&1
status=$?
printf '{"exit_code":%s,"generation_requests":0,"real_auth":0,"authority_deadline_ms":750,"observation_connections":1}\n' "$status" > app-qual-v4r1-exit.json
tail -n 18 app-qual-v4r1.log
exit "$status"

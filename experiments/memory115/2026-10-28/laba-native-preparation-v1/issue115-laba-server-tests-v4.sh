#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e server-tests-v4.log
mkdir host-bin-v4
chmod -R a-w source-server-preparation-v4
(cd source-server-preparation-v4; find . -type f -print0 | sort -z | xargs -0 sha256sum) > server-source-before-v4.sha256
set +e
docker run --rm --name issue115-server-tests-20261028-v2 --network container:issue115-operator-pg-20261028-v1 --cpus 2 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=256m --mount type=bind,src="$base/source-server-preparation-v4",dst=/source,readonly --mount type=bind,src="$base/host-bin-v4",dst=/out --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$base/cache",dst=/cache --mount type=bind,src="$base/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=2 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -e BUTLER_TEST_OPERATOR_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_operator_test?sslmode=disable' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
go test -race -count=1 -v -run="^Test(Laba115SuccessorKeepsHistoryAndRejectsCrossHostClaims|Local115SkillPreparationSuccessorPreservesLatestFailedPopulation|ExperienceNativePresealPinsSourceArtifactsHistoryAndDenominator)$" ./apps/orchestrator/internal/app
go test -race -count=1 -v scripts/dev/memory115-laba-host.go scripts/dev/memory115-laba-auth.go scripts/dev/memory115-laba-execute.go scripts/dev/memory115-laba-host_test.go scripts/dev/memory115-laba-execute_test.go scripts/dev/memory115-local-execute_integration_test.go
go vet ./apps/orchestrator/internal/app
go vet scripts/dev/memory115-laba-host.go scripts/dev/memory115-laba-auth.go scripts/dev/memory115-laba-execute.go
CGO_ENABLED=0 go build -o /out/memory115-laba-execute scripts/dev/memory115-laba-host.go scripts/dev/memory115-laba-auth.go scripts/dev/memory115-laba-execute.go
chmod 0555 /out/memory115-laba-execute
sha256sum /out/memory115-laba-execute
' > server-tests-v4.log 2>&1
status=$?
(cd source-server-preparation-v4; find . -type f -print0 | sort -z | xargs -0 sha256sum) > server-source-after-v4.sha256
cmp server-source-before-v4.sha256 server-source-after-v4.sha256
source_status=$?
printf '{"exit_code":%s,"source_after_exit":%s,"source_state":"unfrozen_server_preparation_v4_not_a58","native_calls":0,"real_auth":0}\n' "$status" "$source_status" > server-tests-v4-exit.json
exit "$status"

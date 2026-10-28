#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e server-lineage-v1.log
chmod -R a-w source-server-preparation-v1
set +e
docker run --rm --name issue115-server-lineage-v1-20261028-v1 --network none --cpus 2 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=256m --mount type=bind,src="$base/source-server-preparation-v1",dst=/source,readonly --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$base/cache",dst=/cache --mount type=bind,src="$base/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=2 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 go test -race -count=1 -v -run='^Test(Laba115SuccessorKeepsHistoryAndRejectsCrossHostClaims|Local115SkillPreparationSuccessorPreservesLatestFailedPopulation|ExperienceNativePresealPinsSourceArtifactsHistoryAndDenominator)$' ./apps/orchestrator/internal/app > server-lineage-v1.log 2>&1
status=$?
printf '{"exit_code":%s,"host":"ssh laba","native_calls":0,"real_auth":0,"source_commit":"a58aa96936c01ee3f9de6d6601e02f8df9058653"}\n' "$status" > server-lineage-v1-exit.json
exit "$status"

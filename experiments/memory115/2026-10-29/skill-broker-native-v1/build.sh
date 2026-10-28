#!/bin/sh
set -eu
umask 077
r=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
p=$r/skill-broker-20261029-v1
c=/srv/gitea/butler-acceptance/issue115-20261028-v1
docker run --rm --name skill-broker-build-20261029-v1 --network none --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m -v "$r/source-abc-v6:/source:ro" -v "$p:/probe" -v "$p/out:/out" -v /srv/storage/butler/.cache/go-mod:/modules:ro -v "$c/cache:/cache" -v "$c/build-tmp:/build-tmp" -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
gofmt -w /probe/delta/*.go
go test -race -overlay=/probe/overlay.json -run="Test(Skill|RecoveredSkills|RecoveredCatalog|PackageSchemaDescriptions)" -count=1 ./apps/orchestrator/internal/orchestrator
go vet -overlay=/probe/overlay.json ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app
if test ! -f /out/plan.json; then
  BUTLER_SKILL_BROKER_EXPORT=1 go test -overlay=/probe/overlay.json -run="^TestSkillBrokerNativePlan$" -count=1 ./apps/orchestrator/internal/app
fi
go test -race -overlay=/probe/overlay.json -c -o /probe/bin/app.test ./apps/orchestrator/internal/app
chmod 0555 /probe/bin/app.test
sha256sum /probe/bin/app.test /out/plan.json
' > "$p/build.log" 2>&1
cat "$p/build.log"

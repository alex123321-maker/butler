#!/bin/sh
set -eu
umask 077
r=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
p=$r/skill-context-20261029-v1
c=/srv/gitea/butler-acceptance/issue115-20261028-v1
printf '%s\n' '{"Replace":{"/source/internal/transport/antigravity/skill_context_probe_test.go":"/probe/delta/skill_context_probe_test.go","/source/internal/transport/antigravity/skill_prompt_probe_test.go":"/probe/delta/skill_prompt_probe_test.go"}}' > "$p/overlay.json"
docker run --rm --name skill-context-build-20261029-v1 --network none --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m -v "$r/source-abc-v6:/source:ro" -v "$p:/probe" -v /srv/storage/butler/.cache/go-mod:/modules:ro -v "$c/cache:/cache" -v "$c/build-tmp:/build-tmp" -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /bin/sh -eu -c '
gofmt -w /probe/delta/skill_context_probe_test.go
BUTLER_CONTEXT_OFFLINE=1 go test -race -overlay=/probe/overlay.json -run="^TestSkillContextProbeFixture$" -count=1 ./internal/transport/antigravity
go test -race -overlay=/probe/overlay.json -count=1 ./internal/transport/antigravity
go vet -overlay=/probe/overlay.json ./internal/transport/antigravity
go test -race -overlay=/probe/overlay.json -c -o /probe/out/probe.test ./internal/transport/antigravity
chmod 0555 /probe/out/probe.test
sha256sum /probe/out/probe.test /probe/plan.json
' > "$p/build.log" 2>&1
cat "$p/build.log"

#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
grep -q '"exit_code":0' controls-exit.json
test ! -e gates-started-v1
: > gates-started-v1
mkdir plans
cp issue115-laba-*-regex-v1.txt plans/
docker exec issue115-pg-20261028-v1 createdb -U postgres memory115_operator_test
run_stage() {
 stage="$1"; shift
 test ! -e "$stage.log"
 set +e
 docker run --rm --name "issue115-$stage-20261028-v1" --network issue115-20261028-v1 --cpus 2 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=256m --mount type=bind,src="$base/source",dst=/source,readonly --mount type=bind,src="$base/prepared",dst=/prepared,readonly --mount type=bind,src="$base/proof",dst=/proof --mount type=bind,src="$base/history",dst=/history,readonly --mount type=bind,src="$base/plans",dst=/plans,readonly --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$base/cache",dst=/cache --mount type=bind,src="$base/build-tmp",dst=/build-tmp -e TMPDIR=/proof/tmp -e GOTMPDIR=/build-tmp -e GOCACHE=/cache -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=2 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@issue115-pg:5432/memory115_test?sslmode=disable' -e BUTLER_TEST_OPERATOR_POSTGRES_URL='postgres://postgres@issue115-pg:5432/memory115_operator_test?sslmode=disable' -e BUTLER_BROKER_E2E_BINARY=/prepared/tool-broker -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/prepared/broker-control.test -e BUTLER_EXTENSION_E2E_BINARY=/prepared/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/prepared/http.test -e BUTLER_EXPERIENCE_PROCESS_PROOF=/proof/retained -w /source/apps/orchestrator/internal/app sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 "$@" > "$stage.log" 2>&1
 status=$?
 set -e
 printf '{"stage":"%s","exit_code":%s,"native_calls":0,"real_auth":0}\n' "$stage" "$status" > "$stage-exit.json"
 test "$status" = 0
}
run_stage app-controls /bin/sh -eu -c 'exec /prepared/app.test -test.v -test.count=1 -test.timeout=25m -test.run="$(cat /plans/issue115-laba-app-regex-v1.txt)"'
run_stage source-guards /bin/sh -eu -c 'cd /source; go test -race -count=1 -v -timeout=10m -run="$(cat /plans/issue115-laba-orchestrator-regex-v1.txt)" ./apps/orchestrator/internal/orchestrator; go test -race -count=1 -v -timeout=10m -run="$(cat /plans/issue115-laba-catalog-regex-v1.txt)" ./internal/memory/catalog'
run_stage lesson-read /usr/bin/env BUTLER_EXPERIENCE_NATIVE_HISTORY=/history/memory115-skill-successor-recipe-v1/recipe.json BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256=370e4c2ae13fb25ca4a4dbb9891464959df57a4b9bb7ea0ad0a4d25888ddabe4 BUTLER_ISSUE115_SKILL_SUCCESSOR_READ=fresh-scripted-latest232-v1 /prepared/app.test -test.v -test.count=1 -test.timeout=25m -test.run='^TestExperienceNativeSkillPreparationSuccessorReadIntegration$'
run_stage operator /bin/sh -eu -c 'cd /source; go test -race -count=1 -v -timeout=3m scripts/dev/memory115-local-execute.go scripts/dev/memory115-local-execute_test.go scripts/dev/memory115-local-execute_integration_test.go'
run_stage vet /bin/sh -eu -c 'cd /source; go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator ./internal/memory/catalog'
(cd source; sha256sum -c ../source.sha256 > ../source-after-gates.log)
printf '{"all_stage_exits_zero":true,"native_calls":0,"real_auth":0,"host":"ssh laba"}\n' > gates-exit.json

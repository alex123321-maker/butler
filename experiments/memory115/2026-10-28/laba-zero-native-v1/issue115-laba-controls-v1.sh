#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
grep -q '"exit_code":0' build-exit.json
test ! -e controls.log
mkdir -p proof/tmp
(cd source; sha256sum -c ../source.sha256 > ../source-before-controls.log)
set +e
docker run --rm --name issue115-controls-20261028-v1 --network issue115-20261028-v1 --cpus 2 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=256m --mount type=bind,src="$base/source",dst=/source,readonly --mount type=bind,src="$base/prepared",dst=/prepared,readonly --mount type=bind,src="$base/proof",dst=/proof -e TMPDIR=/proof/tmp -e GOMAXPROCS=2 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@issue115-pg:5432/memory115_test?sslmode=disable' -e BUTLER_BROKER_E2E_BINARY=/prepared/tool-broker -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/prepared/broker-control.test -e BUTLER_EXTENSION_E2E_BINARY=/prepared/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/prepared/http.test -e BUTLER_EXPERIENCE_PROCESS_PROOF=/proof/controlled -w /source/apps/orchestrator/internal/app sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /prepared/app.test -test.v -test.count=1 -test.timeout=25m -test.run='^TestExperienceNativeWholeCollectorControlledIntegration$' > controls.log 2>&1
status=$?
(cd source; sha256sum -c ../source.sha256 > ../source-after-controls.log)
source_status=$?
printf '{"exit_code":%s,"source_after_exit":%s,"host":"ssh laba","native_calls":0,"real_auth":0,"source_commit":"a58aa96936c01ee3f9de6d6601e02f8df9058653"}\n' "$status" "$source_status" > controls-exit.json
exit "$status"

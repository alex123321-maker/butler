#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cd "$base"
grep -q '"exit_code":1' bound-native-proxy-v2-exit.json
grep -q '"all_private_credential_backends_joined": true' operator-native-proxy-v2/server-handback.json
grep -q '"whole_cgroup_empty": true' operator-native-proxy-v2/process-exit.json
grep -q '"forced_cleanup": false' operator-native-proxy-v2/process-exit.json
test ! -e review-snapshot-v1
mkdir review-snapshot-v1
cp -a operator-native-proxy-v2/execution/actual review-snapshot-v1/actual
cp prepared-proxy-v2/plan/preseal.json review-snapshot-v1/preseal.json
cp operator-native-proxy-v2/runner.log review-snapshot-v1/runner.log
cp operator-native-proxy-v2/process-exit.json operator-native-proxy-v2/server-handback.json review-snapshot-v1/
(cd review-snapshot-v1; find . -type f -print0 | sort -z | xargs -0 sha256sum) > private-native-raw-inventory-v1.sha256
tar -cf private-native-raw-v1.tar -C review-snapshot-v1 .
sha256sum private-native-raw-v1.tar private-native-raw-inventory-v1.sha256 > private-native-seals-v1.sha256
chmod -R a-w review-snapshot-v1
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
set +e
d run --rm --name issue115-cost-review-v1 --network none --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m --mount type=bind,src="$base/source-proxy-v2",dst=/source,readonly --mount type=bind,src="$base/review-bin-v1",dst=/review-bin,readonly --mount type=bind,src="$base/review-snapshot-v1",dst=/review,readonly --mount type=bind,src="$base/history",dst=/history,readonly --mount type=bind,src="$base/review-output-v1",dst=/review-output -e GOMAXPROCS=1 -e BUTLER_ISSUE115_LABA_COST_REVIEW=closed-proxy-v2 -w /source/apps/orchestrator/internal/app sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /review-bin/review.test -test.v -test.count=1 -test.run='^TestLaba115ClosedCostReview$' -test.timeout=3m > review-run-v1.log 2>&1
status=$?
printf '{"exit_code":%s,"generation_requests":0,"network":"none","post_exit_reader_only":true}\n' "$status" > review-run-v1-exit.json
cat review-run-v1.log
exit "$status"
